package server

import (
	"context"

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/db"
)

func (s JamsyncServer) AddProject(ctx context.Context, in *pb.AddProjectRequest) (*pb.AddProjectResponse, error) {
	projectId, err := db.AddProject(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}

	return &pb.AddProjectResponse{
		ProjectId: projectId,
	}, nil
}

func (s JamsyncServer) ListProjects(ctx context.Context, in *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	projects, err := db.ListProjects(s.db)
	if err != nil {
		return nil, err
	}

	projectsPb := make([]*pb.ListProjectsResponse_Project, len(projects))
	for i := range projectsPb {
		projectsPb[i] = &pb.ListProjectsResponse_Project{Name: projects[i].Name, Id: projects[i].Id}
	}

	return &pb.ListProjectsResponse{Projects: projectsPb}, nil
}

func (s JamsyncServer) GetProjectConfig(ctx context.Context, in *pb.GetProjectConfigRequest) (*pb.ProjectConfig, error) {
	projectId, err := db.GetProjectId(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}

	changeId, _, err := db.GetCurrentChange(s.db, projectId)
	if err != nil {
		return nil, err
	}

	return &pb.ProjectConfig{ProjectId: projectId, CurrentChange: changeId}, nil
}
