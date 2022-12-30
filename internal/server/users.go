package server

import (
	"context"

	"github.com/zdgeier/jamsync/gen/pb"
)

func (s JamsyncServer) CreateUser(ctx context.Context, in *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	_, err := s.db.CreateUser(in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &pb.CreateUserResponse{}, nil
}
