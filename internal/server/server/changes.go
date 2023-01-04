package server

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/rsync"
	"github.com/zdgeier/jamsync/internal/server/serverauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (s JamsyncServer) CreateChange(ctx context.Context, in *pb.CreateChangeRequest) (*pb.CreateChangeResponse, error) {
	owner, err := s.db.GetProjectOwner(in.GetProjectId())
	if err != nil {
		return nil, err
	}
	userId, err := serverauth.ParseIdFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if owner != userId {
		return nil, status.Errorf(codes.Unauthenticated, "unauthorized")
	}

	changeId, err := s.changestore.AddChange(in.GetProjectId())
	if err != nil {
		return nil, err
	}
	return &pb.CreateChangeResponse{
		ChangeId: changeId,
	}, nil
}

func (s JamsyncServer) WriteOperationStream(srv pb.JamsyncAPI_WriteOperationStreamServer) error {
	userId, err := serverauth.ParseIdFromCtx(srv.Context())
	if err != nil {
		return err
	}

	operationProject := uint64(0)
	var projectId, changeId, pathHash uint64
	opLocs := make([]*pb.OperationLocations_OperationLocation, 0)
	for {
		in, err := srv.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		data, err := proto.Marshal(in)
		if err != nil {
			return err
		}
		projectId = in.GetProjectId()
		changeId = in.GetChangeId()
		pathHash = in.GetPathHash()
		if operationProject == 0 {
			owner, err := s.db.GetProjectOwner(projectId)
			if err != nil {
				return err
			}
			if userId != owner {
				return status.Errorf(codes.Unauthenticated, "unauthorized")
			}
			operationProject = projectId
		}

		if operationProject != projectId {
			return status.Errorf(codes.Unauthenticated, "unauthorized")
		}

		offset, length, err := s.opstore.Write(projectId, changeId, pathHash, data)
		if err != nil {
			return err
		}
		operationLocation := &pb.OperationLocations_OperationLocation{
			Offset: offset,
			Length: length,
		}
		opLocs = append(opLocs, operationLocation)
	}
	err = s.oplocstore.InsertOperationLocations(&pb.OperationLocations{
		ProjectId: projectId,
		ChangeId:  changeId,
		PathHash:  pathHash,
		OpLocs:    opLocs,
	})
	if err != nil {
		return err
	}

	return srv.SendAndClose(&pb.WriteOperationStreamResponse{})
}

func (s JamsyncServer) ReadBlockHashes(ctx context.Context, in *pb.ReadBlockHashesRequest) (*pb.ReadBlockHashesResponse, error) {
	owner, err := s.db.GetProjectOwner(in.GetProjectId())
	if err != nil {
		return nil, err
	}
	userId, err := serverauth.ParseIdFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if owner != userId {
		return nil, status.Errorf(codes.Unauthenticated, "unauthorized")
	}

	targetBuffer, err := s.regenFile(in.GetProjectId(), in.GetPathHash(), in.GetModTime().AsTime())
	if err != nil {
		return nil, err
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	sig := make([]*pb.BlockHash, 0)
	err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
		sig = append(sig, &pb.BlockHash{
			Index:      bl.Index,
			StrongHash: bl.StrongHash,
			WeakHash:   bl.WeakHash,
		})
		return nil
	})
	return &pb.ReadBlockHashesResponse{
		BlockHashes: sig,
	}, err
}

func (s JamsyncServer) regenFile(projectId uint64, pathHash uint64, modTime time.Time) (*bytes.Reader, error) {
	changeIds, err := s.changestore.ListCommittedChanges(projectId, modTime)
	if err != nil {
		return nil, err
	}

	uniqueChangeIds := make(map[uint64]interface{}, 0)
	for _, id := range changeIds {
		uniqueChangeIds[id] = nil
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewBuffer([]byte{})
	result := new(bytes.Buffer)
	for _, changeId := range changeIds {
		operationLocations, err := s.oplocstore.ListOperationLocations(projectId, pathHash, changeId)
		if err != nil {
			return nil, err
		}
		if operationLocations == nil {
			continue
		}
		ops := make([]rsync.Operation, 0, len(operationLocations.GetOpLocs()))
		for _, loc := range operationLocations.GetOpLocs() {
			b, err := s.opstore.Read(projectId, operationLocations.ChangeId, pathHash, loc.GetOffset(), loc.GetLength())
			if err != nil {
				panic(err)
			}

			op := new(pb.Operation)
			err = proto.Unmarshal(b, op)
			if err != nil {
				panic(err)
			}
			ops = append(ops, rsync.PbOperationToRsync(op))
		}
		err = rs.ApplyDeltaBatch(result, bytes.NewReader(targetBuffer.Bytes()), ops)
		if err != nil {
			panic(err)
		}
		targetBuffer.Reset()
		targetBuffer.Write(result.Bytes())
		result.Reset()
	}
	return bytes.NewReader(targetBuffer.Bytes()), nil
}

func (s JamsyncServer) ReadFile(in *pb.ReadFileRequest, srv pb.JamsyncAPI_ReadFileServer) error {
	owner, err := s.db.GetProjectOwner(in.GetProjectId())
	if err != nil {
		return err
	}
	userId, err := serverauth.ParseIdFromCtx(srv.Context())
	if err != nil {
		return err
	}
	if owner != userId {
		return status.Errorf(codes.Unauthenticated, "unauthorized")
	}

	sourceBuffer, err := s.regenFile(in.GetProjectId(), in.GetPathHash(), in.GetModTime().AsTime())
	if err != nil {
		return err
	}

	//a, _ := io.ReadAll(sourceBuffer)
	//fmt.Println("READING", in.PathHash, string(a))
	//sourceBuffer.Seek(0, 0)

	opsOut := make(chan *rsync.Operation)
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}
	go func() {
		var blockCt, blockRangeCt, dataCt, bytes int
		defer close(opsOut)
		err := rsDelta.CreateDelta(sourceBuffer, rsync.PbBlockHashesToRsync(in.GetBlockHashes()), func(op rsync.Operation) error {
			switch op.Type {
			case rsync.OpBlockRange:
				blockRangeCt++
			case rsync.OpBlock:
				blockCt++
			case rsync.OpData:
				// Copy data buffer so it may be reused in internal buffer.
				b := make([]byte, len(op.Data))
				copy(b, op.Data)
				op.Data = b
				dataCt++
				bytes += len(op.Data)
			}
			opsOut <- &op
			return nil
		})
		if err != nil {
			panic(err)
		}
	}()

	for op := range opsOut {
		var opPbType pb.Operation_Type
		switch op.Type {
		case rsync.OpBlock:
			opPbType = pb.Operation_OpBlock
		case rsync.OpData:
			opPbType = pb.Operation_OpData
		case rsync.OpHash:
			opPbType = pb.Operation_OpHash
		case rsync.OpBlockRange:
			opPbType = pb.Operation_OpBlockRange
		}

		err = srv.Send(&pb.Operation{
			ProjectId:     in.GetProjectId(),
			ChangeId:      in.GetChangeId(),
			PathHash:      in.GetPathHash(),
			Type:          opPbType,
			BlockIndex:    op.BlockIndex,
			BlockIndexEnd: op.BlockIndexEnd,
			Data:          op.Data,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s JamsyncServer) CommitChange(ctx context.Context, in *pb.CommitChangeRequest) (*pb.CommitChangeResponse, error) {
	owner, err := s.db.GetProjectOwner(in.GetProjectId())
	if err != nil {
		return nil, err
	}
	userId, err := serverauth.ParseIdFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if owner != userId {
		return nil, status.Errorf(codes.Unauthenticated, "unauthorized")
	}

	err = s.changestore.CommitChange(in.GetProjectId(), in.GetChangeId())
	if err != nil {
		return nil, err
	}
	return &pb.CommitChangeResponse{}, nil
}
