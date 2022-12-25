package server

import (
	"context"

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/db"
)

func (s JamsyncServer) CreateUser(ctx context.Context, in *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &pb.CreateUserResponse{}, nil
}
