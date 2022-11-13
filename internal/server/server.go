package server

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func (s JamsyncServer) AddProject(ctx context.Context, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	res, err := db.AddProject(s.db, in.GetName(), in.GetOwner())
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.AddProjectResponse{ProjectId: res}, nil
}

func (s JamsyncServer) GetProject(ctx context.Context, in *jamsyncpb.GetProjectRequest) (*jamsyncpb.GetProjectResponse, error) {
	name, res, err := db.GetProject(s.db, in.GetProjectId())
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetProjectResponse{Name: name, Owner: res}, nil
}

func (s JamsyncServer) ListProjects(ctx context.Context, in *jamsyncpb.ListProjectsRequest) (*jamsyncpb.ListProjectsResponse, error) {
	projects, err := db.ListProjects(s.db)
	if err != nil {
		return nil, err
	}

	projectsPb := make([]*jamsyncpb.ListProjectsResponse_Project, len(projects))
	for i := range projectsPb {
		projectsPb[i] = &jamsyncpb.ListProjectsResponse_Project{Name: projects[i].Name, OwnerUserId: projects[i].OwnerId, Id: projects[i].Id}
	}

	return &jamsyncpb.ListProjectsResponse{Projects: projectsPb}, nil
}

func (s JamsyncServer) AddUser(ctx context.Context, in *jamsyncpb.AddUserRequest) (*jamsyncpb.AddUserResponse, error) {
	id, err := db.AddUser(s.db, in.Username)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.AddUserResponse{UserId: id}, nil
}

func (s JamsyncServer) GetUser(ctx context.Context, in *jamsyncpb.GetUserRequest) (*jamsyncpb.GetUserResponse, error) {
	id, err := db.GetUser(s.db, in.GetUserId())
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetUserResponse{Username: id}, nil
}

func (s JamsyncServer) ListUsers(ctx context.Context, in *jamsyncpb.ListUsersRequest) (*jamsyncpb.ListUsersResponse, error) {
	users, err := db.ListUsers(s.db)
	if err != nil {
		return nil, err
	}

	usersPb := make([]*jamsyncpb.ListUsersResponse_User, len(users))
	for i := range usersPb {
		usersPb[i] = &jamsyncpb.ListUsersResponse_User{Username: users[i].Username, UserId: users[i].Id}
	}

	return &jamsyncpb.ListUsersResponse{Users: usersPb}, nil
}

func (s JamsyncServer) UpdateStream(stream jamsyncpb.JamsyncAPI_UpdateStreamServer) error {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Println(string(in.Data))
	}
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,
	}

	return server
}

func (s JamsyncServer) GenTestData() {
	id, _ := db.AddUser(s.db, "zdgeier")
	db.AddUser(s.db, "testuser1")
	db.AddUser(s.db, "testuser2")
	db.AddUser(s.db, "testuser3")
	db.AddUser(s.db, "testuser4")
	db.AddProject(s.db, "TestProject", id)
	db.AddProject(s.db, "Jamsync", id)
	db.AddProject(s.db, "JamsyncOpen", id)
}
