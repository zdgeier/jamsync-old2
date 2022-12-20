package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"google.golang.org/grpc/status"
)

func pathToHash(path string) uint64 {
	h := xxhash.New()
	h.Write([]byte(path))
	return h.Sum64()
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	fmt.Println("regenning", in.Path)
	reader, err := s.regenFile(ctx, in.ProjectName, pathToHash(in.Path), in.Timestamp.AsTime())
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	fmt.Println("GOT", data)
	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
}

func (s JamsyncServer) regenFile(ctx context.Context, projectName string, pathHash uint64, timestamp time.Time) (*bytes.Reader, error) {
	changeLocationLists, err := db.ChangeLocationLists(s.db, projectName, pathHash, timestamp)
	if err != nil {
		return nil, err
	}
	fmt.Println(pathHash)

	dataClient, err := s.storeClient.ReadChangeData(ctx)
	if err != nil {
		return nil, err
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, changeLocationList := range changeLocationLists {
		fmt.Println("sendlist", changeLocationList)
		err := dataClient.Send(changeLocationList)
		if err != nil {
			return nil, err
		}

		ops := make(chan rsync.Operation)
		go func() {
			for {
				in, err := dataClient.Recv()
				if err == io.EOF {
					// read done.
					close(ops)
					break
				}
				if err != nil {
					log.Panic(err)
				}

				ops <- pbOperationToRsync(in)
			}
		}()

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

	for _, file := range in.FileList.GetFiles() {
		log.Println("getting", file.Path)
		if file.Dir {
			continue
		}
		resp, err := s.GetFile(context.TODO(), &jamsyncpb.GetFileRequest{
			ProjectName: projectName,
			Path:        file.GetPath(),
			Timestamp:   in.Timestamp,
		})
		if err != nil {
			return status.Error(400, err.Error())
		}

		reader := bytes.NewReader(resp.GetData())

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

		if err := srv.Send(&jamsyncpb.GetFileBlockHashesResponse{
			Path:        file.GetPath(),
			BlockHashes: blockHashesPb,
		}); err != nil {
			return err
		}
	}
	return nil
}
