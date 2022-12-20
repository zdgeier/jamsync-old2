package server

import (
	"bytes"
	"context"
	"io"
	"log"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"google.golang.org/grpc/status"
)

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	reader, err := s.regenFile(ctx, in.ProjectName, in.Path, in.Timestamp.AsTime())
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
}

func (s JamsyncServer) regenFile(ctx context.Context, projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	changeLocationLists, err := db.ChangeLocationLists(s.db, projectName, path, timestamp)
	if err != nil {
		return nil, err
	}

	dataClient, err := s.storeClient.ReadChangeData(ctx)
	if err != nil {
		return nil, err
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, changeLocationList := range changeLocationLists {
		err := dataClient.Send(changeLocationList)
		if err != nil {
			return nil, err
		}

		ops := make(chan rsync.Operation)
		for {
			in, err := dataClient.Recv()
			if err == io.EOF {
				// read done.
				close(ops)
				break
			}
			if err != nil {
				return nil, err
			}

			ops <- pbOperationToRsync(in)
		}

		err = rs.ApplyDelta(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()

	for _, path := range in.GetPaths() {
		log.Println("getting", path)
		resp, err := s.GetFile(context.TODO(), &jamsyncpb.GetFileRequest{
			ProjectName: projectName,
			Path:        path,
			Timestamp:   in.Timestamp,
		})
		if err != nil {
			return status.Error(400, err.Error())
		}

		reader := bytes.NewReader(resp.GetData())

		log.Println("makinggg", path)
		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(reader, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			return err
		}

		log.Println("sending", path)
		if err := srv.Send(&jamsyncpb.GetFileBlockHashesResponse{
			Path:        path,
			BlockHashes: blockHashesPb,
		}); err != nil {
			return err
		}
	}
	return nil
}
