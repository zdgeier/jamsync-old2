package server

import (
	"context"

	"github.com/zdgeier/jamsync/gen/pb"
)

func (s JamsyncServer) AddProject(ctx context.Context, in *pb.AddProjectRequest) (*pb.AddProjectResponse, error) {
	projectId, err := s.db.AddProject(in.GetProjectName(), in.GetOwner())
	if err != nil {
		return nil, err
	}

	return &pb.AddProjectResponse{
		ProjectId: projectId,
	}, nil
}

func (s JamsyncServer) ListUserProjects(ctx context.Context, in *pb.ListUserProjectsRequest) (*pb.ListUserProjectsResponse, error) {
	projects, err := s.db.ListUserProjects(in.GetOwner())
	if err != nil {
		return nil, err
	}

	projectsPb := make([]*pb.ListUserProjectsResponse_Project, len(projects))
	for i := range projectsPb {
		projectsPb[i] = &pb.ListUserProjectsResponse_Project{Name: projects[i].Name, Id: projects[i].Id}
	}

	return &pb.ListUserProjectsResponse{Projects: projectsPb}, nil
}

func (s JamsyncServer) ListProjects(ctx context.Context, in *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	projects, err := s.db.ListProjects()
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
	projectId, err := s.db.GetProjectId(in.GetProjectName())
	if err != nil {
		return nil, err
	}

	changeId, _, err := s.changestore.GetCurrentChange(projectId)
	if err != nil {
		return nil, err
	}

	return &pb.ProjectConfig{ProjectId: projectId, CurrentChange: changeId}, nil
}
