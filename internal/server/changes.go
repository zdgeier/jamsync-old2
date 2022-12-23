package server

import (
	"context"
	"fmt"
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
	waitc := make(chan struct{})
	go func() {
		for {
			// TODO: handle errors in here!
			fmt.Println("PdjkOPstream")
			in, err := opStream.Recv()
			if err == io.EOF {
				close(waitc)
				return
			}
			if err != nil {
				return
			}
			fmt.Println("LIST", in.String())

			_, err = db.AddChangeLocationList(s.db, in)
			if err != nil {
				return
			}
		}
	}()

	for {
		fmt.Println("POPstream")
		op, err := srv.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fmt.Println("OP", op)
		err = opStream.Send(op)
		if err != nil {
			return err
		}
	}
	opStream.CloseSend()
	<-waitc

	return nil
}
