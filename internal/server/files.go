package server

import (
	"context"
	"io"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/changestore"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
)

const (
	fileListPath = ".jamsyncfilelist"
)

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	changeLocations, err := db.ListChangeDataLocations(s.db, in.GetProjectName(), in.GetPath())
	if err != nil {
		return nil, err
	}
	reader, err := changestore.RegenFile(s.store, in.GetProjectName(), in.GetPath(), changeLocations)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	changeLocations, err := db.ListChangeDataLocations(s.db, in.GetProjectName(), fileListPath)
	if err != nil {
		return nil, err
	}
	reader, err := changestore.RegenFile(s.store, in.GetProjectName(), fileListPath, changeLocations)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	// log.Println("GetFileList")
	// targetBuffer, err := s.regenFile(in.GetProjectName(), "jamsyncfilelist", time.Now())
	// if err != nil {
	// 	return nil, err
	// }

	// data, err := io.ReadAll(targetBuffer)
	// if err != nil {
	// 	return nil, err
	// }

	// files := &jamsyncpb.GetFileListResponse{}
	// err = proto.Unmarshal(data, files)
	// if err != nil {
	// 	return nil, err
	// }

	return files, nil
}

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		changeLocations, err := db.ListChangeDataLocations(s.db, projectName, path)
		if err != nil {
			return err
		}
		reader, err := changestore.RegenFile(s.store, projectName, path, changeLocations)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(reader, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			return err
		}

		if err := srv.Send(&jamsyncpb.GetFileBlockHashesResponse{
			Path:        path,
			BlockHashes: blockHashesPb,
		}); err != nil {
			return err
		}
	}
	return nil
}
