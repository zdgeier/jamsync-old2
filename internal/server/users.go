package server

import (
	"context"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
)

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
}
