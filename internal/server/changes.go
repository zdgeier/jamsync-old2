package server

import (
	"context"
	"io"
	"log"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		changeId uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := s.store.WriteChangeData(in.GetProjectName(), in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}
