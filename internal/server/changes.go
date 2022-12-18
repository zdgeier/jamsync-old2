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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	changeId, timestamp, err := db.GetCurrentChange(tx, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, tx.Commit()
}

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	var changeId uint64
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
			changeId, err = db.AddChange(tx, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		changeLocation, err := s.store.WriteChangeData(in.GetProjectName(), in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			return err
		}

		_, err = db.AddChangeData(tx, changeId, in.GetPath(), changeLocation)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
