package server

import (
	"context"
	"log"
	"path/filepath"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"google.golang.org/protobuf/proto"
)

func (s JamsyncServer) AddProject(ctx context.Context, in *jamsyncpb.AddProjectRequest) (resp *jamsyncpb.AddProjectResponse, err error) {
	log.Println("AddProject")

	_, err = db.AddProject(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.AddProjectResponse{}, nil
}

func (s JamsyncServer) ListProjects(ctx context.Context, in *jamsyncpb.ListProjectsRequest) (*jamsyncpb.ListProjectsResponse, error) {
	projects, err := db.ListProjects(s.db)
	if err != nil {
		return nil, err
	}

	projectsPb := make([]*jamsyncpb.ListProjectsResponse_Project, len(projects))
	for i := range projectsPb {
		projectsPb[i] = &jamsyncpb.ListProjectsResponse_Project{Name: projects[i].Name, Id: projects[i].Id}
	}

	return &jamsyncpb.ListProjectsResponse{Projects: projectsPb}, nil
}

func (s JamsyncServer) BrowseProject(ctx context.Context, in *jamsyncpb.BrowseProjectRequest) (*jamsyncpb.BrowseProjectResponse, error) {
	log.Println("BrowseProject", in.String())
	resp, err := s.GetFile(ctx, &jamsyncpb.GetFileRequest{
		ProjectName: in.ProjectName,
		Path:        ".jamsyncfilelist",
	})
	if err != nil {
		return nil, err
	}

	files := &jamsyncpb.FileList{}
	err = proto.Unmarshal(resp.GetData(), files)
	if err != nil {
		return nil, err
	}

	directoryNames := make([]string, 0, len(files.GetFiles()))
	fileNames := make([]string, 0, len(files.GetFiles()))
	requestPath := filepath.Clean(in.GetPath())
	for _, file := range files.GetFiles() {
		pathDir := filepath.Dir(file.GetPath())
		if (in.GetPath() == "" && pathDir == ".") || pathDir == requestPath {
			if file.Dir {
				directoryNames = append(directoryNames, filepath.Base(file.GetPath()))
			} else {
				fileNames = append(fileNames, filepath.Base(file.GetPath()))
			}
		}
	}

	return &jamsyncpb.BrowseProjectResponse{
		Directories: directoryNames,
		Files:       fileNames,
	}, nil
}
