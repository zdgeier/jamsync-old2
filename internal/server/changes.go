package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"google.golang.org/protobuf/proto"
)

func (s JamsyncServer) CreateChange(ctx context.Context, in *pb.CreateChangeRequest) (*pb.CreateChangeResponse, error) {
	changeId, err := db.AddChange(s.db, in.GetProjectId())
	if err != nil {
		return nil, err
	}
	return &pb.CreateChangeResponse{
		ChangeId: changeId,
	}, nil
}

func (s JamsyncServer) WriteOperationStream(srv pb.JamsyncAPI_WriteOperationStreamServer) error {
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
		offset, length, err := s.store.Write(in.GetProjectId(), in.GetChangeId(), in.GetPathHash(), data)
		if err != nil {
			return err
		}
		operationLocation := &pb.OperationLocation{
			ProjectId: in.GetProjectId(),
			ChangeId:  in.GetChangeId(),
			PathHash:  in.GetPathHash(),
			Offset:    offset,
			Length:    length,
		}
		_, err = db.AddOperationLocation(s.db, operationLocation)
		if err != nil {
			return err
		}
	}

	return srv.SendAndClose(&pb.WriteOperationStreamResponse{})
}

func (s JamsyncServer) ReadOperationStream(srv pb.JamsyncAPI_ReadOperationStreamServer) error {
	for {
		in, err := srv.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		b, err := s.store.Read(in.GetProjectId(), in.GetChangeId(), in.GetPathHash(), in.GetOffset(), in.GetLength())
		if err != nil {
			return err
		}

		op := new(pb.Operation)
		err = proto.Unmarshal(b, op)
		if err != nil {
			return err
		}
		err = srv.Send(op)
		if err != nil {
			return err
		}
	}
}

func (s JamsyncServer) ReadBlockHashes(ctx context.Context, in *pb.ReadBlockHashesRequest) (*pb.ReadBlockHashesResponse, error) {
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
	changeIds, err := db.ListCommittedChanges(s.db, projectId, pathHash, modTime)
	if err != nil {
		return nil, err
	}
	fmt.Println("COMMITTED", pathHash, changeIds, modTime)

	uniqueChangeIds := make(map[uint64]interface{}, 0)
	for _, id := range changeIds {
		uniqueChangeIds[id] = nil
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewBuffer([]byte{})
	result := new(bytes.Buffer)
	appliedChanges := make(map[uint64]interface{}, 0)
	for _, changeId := range changeIds {
		if _, found := appliedChanges[changeId]; found {
			continue
		}
		appliedChanges[changeId] = nil
		operationLocations, err := db.ListOperationLocations(s.db, projectId, pathHash, changeId)
		if err != nil {
			return nil, err
		}
		ops := make(chan rsync.Operation)
		go func() {
			for _, loc := range operationLocations {
				b, err := s.store.Read(loc.GetProjectId(), loc.GetChangeId(), loc.GetPathHash(), loc.GetOffset(), loc.GetLength())
				if err != nil {
					panic(err)
				}

				op := new(pb.Operation)
				err = proto.Unmarshal(b, op)
				if err != nil {
					panic(err)
				}
				ops <- PbOperationToRsync(op)
			}
			close(ops)
		}()
		err = rs.ApplyDelta(result, bytes.NewReader(targetBuffer.Bytes()), ops)
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
	sourceBuffer, err := s.regenFile(in.GetProjectId(), in.GetPathHash(), in.GetModTime().AsTime())
	if err != nil {
		return err
	}

	a, _ := io.ReadAll(sourceBuffer)
	fmt.Println("READING", in.PathHash, string(a))
	sourceBuffer.Seek(0, 0)

	opsOut := make(chan *rsync.Operation)
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}
	go func() {
		var blockCt, blockRangeCt, dataCt, bytes int
		defer close(opsOut)
		err := rsDelta.CreateDelta(sourceBuffer, PbBlockHashesToRsync(in.GetBlockHashes()), func(op rsync.Operation) error {
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

func PbBlockHashesToRsync(pbBlockHashes []*pb.BlockHash) []rsync.BlockHash {
	blockHashes := make([]rsync.BlockHash, 0)
	for _, pbBlockHash := range pbBlockHashes {
		blockHashes = append(blockHashes, rsync.BlockHash{
			Index:      pbBlockHash.GetIndex(),
			StrongHash: pbBlockHash.GetStrongHash(),
			WeakHash:   pbBlockHash.GetWeakHash(),
		})
	}
	return blockHashes
}

func PbOperationToRsync(op *pb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case pb.Operation_OpBlock:
		opType = rsync.OpBlock
	case pb.Operation_OpData:
		opType = rsync.OpData
	case pb.Operation_OpHash:
		opType = rsync.OpHash
	case pb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) CommitChange(ctx context.Context, in *pb.CommitChangeRequest) (*pb.CommitChangeResponse, error) {
	err := db.CommitChange(s.db, in.GetProjectId(), in.GetChangeId())
	if err != nil {
		return nil, err
	}
	return &pb.CommitChangeResponse{}, nil
}
