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

func (s JamsyncServer) CreateChange(ctx context.Context, in *jamsyncpb.CreateChangeRequest) (*jamsyncpb.CreateChangeResponse, error) {
	projectId, err := db.GetProjectId(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	changeId, err := db.AddChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	_, err = s.storeClient.CreateChangeDir(ctx, &jamsyncpb.CreateChangeDirRequest{
		ProjectId: projectId,
		ChangeId:  changeId,
	})
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.CreateChangeResponse{
		ProjectId: projectId,
		ChangeId:  changeId,
	}, nil
}

func (s JamsyncServer) StreamChange(srv jamsyncpb.JamsyncAPI_StreamChangeServer) error {
	opStream, err := s.storeClient.StreamChange(context.TODO())
	if err != nil {
		return err
	}
	go func() {
		for {
			op, err := srv.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Fatalf("Failed to receive a note : %v", err)
			}
			err = opStream.Send(op)
			if err != nil {
				log.Fatal("could not send data")
			}
		}
	}()

	for {
		in, err := opStream.Recv()
		if err == io.EOF {
			return err
		}
		if err != nil {
			return err
		}

		_, err = db.AddChangeLocationList(s.db, in)
		if err != nil {
			return err
		}
	}
}
