package server

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"time"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s JamsyncServer) AddProject(ctx context.Context, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProject", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	_, err := db.AddProject(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}

	changeId, err := db.AddChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}

	fileListData, err := proto.Marshal(in.ExistingFiles)
	if err != nil {
		return nil, err
	}

	offset, length, err := s.store.WriteFile(in.GetProjectName(), "jamsyncfilelist", fileListData)
	if err != nil {
		return nil, err
	}
	_, err = db.AddChangeData(s.db, changeId, "jamsyncfilelist", offset, length)
	if err != nil {
		return nil, err
	}

	type metadata struct {
		offset int64
		length int
		file   *jamsyncpb.File
	}

	writeFiles := func() ([]metadata, error) {
		g := new(errgroup.Group)

		res := make([]metadata, 0, len(in.GetExistingFiles().Files))
		for i, file := range in.GetExistingFiles().Files {
			fileRef := file
			dataIndex := i
			g.Go(func() error {
				if !fileRef.Dir {
					offset, length, err := s.store.WriteFile(in.GetProjectName(), fileRef.GetPath(), in.GetExistingData()[dataIndex])
					if err != nil {
						return err
					}
					res = append(res, metadata{offset, length, fileRef})
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
		return res, nil
	}

	fileWriteMetadata, err := writeFiles()
	if err != nil {
		return nil, err
	}

	for _, m := range fileWriteMetadata {
		_, err = db.AddChangeData(s.db, changeId, m.file.Path, m.offset, m.length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.AddProjectResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
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
	targetBuffer, err := s.regenFile(in.GetProjectName(), "jamsyncfilelist", time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	files := &jamsyncpb.GetFileListResponse{}
	err = proto.Unmarshal(data, files)
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
