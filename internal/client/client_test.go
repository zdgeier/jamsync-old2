package client

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/server"
	"github.com/zdgeier/jamsync/internal/store"
)

func TestUploadDownload(t *testing.T) {
	ctx := context.Background()

	localDB, err := sql.Open("sqlite3", "file:foobar0?mode=memory&cache=shared")
	if err != nil {
		log.Panic(err)
	}
	db.Setup(localDB)

	client, closeClient := server.NewLocalServer(ctx, localDB, store.NewMemoryStore())
	defer closeClient()

	projectName := "test"
	// _ := &pb.FileMetadata{
	// 	Files: map[string]*pb.File{
	// 		"test.txt": {
	// 			ModTime: timestamppb.New(time.UnixMilli(100)),
	// 		},
	// 		"testdir": {
	// 			ModTime: timestamppb.New(time.UnixMilli(200)),
	// 			Dir:     true,
	// 		},
	// 		"testdir/test2.txt": {
	// 			ModTime: timestamppb.New(time.UnixMilli(200)),
	// 		},
	// 	},
	// }
	addProjectResp, err := client.AddProject(context.Background(), &pb.AddProjectRequest{
		ProjectName: projectName,
	})
	require.NoError(t, err)
	projectId := addProjectResp.GetProjectId()

	fileOperations := []struct {
		name     string
		filePath string
		data     []byte
	}{
		{
			name:     "test1",
			filePath: "test",
			data:     []byte("this is a test!"),
		},
		{
			name:     "test2",
			filePath: "test2",
			data:     []byte("this is a test!"),
		},
		{
			name:     "new path",
			filePath: "this/is/a/path.txt",
			data:     []byte("this is a test!this is a test!this is a test!this is a test!this is a test!this is a test!"),
		},
		{
			name:     "reused path",
			filePath: "this/is/a/path.txt",
			data:     []byte("xthis is a test!this is a test!this is a test!this is a test!this is a test!this is a test!"),
		},
		{
			name:     "reused path",
			filePath: "this/is/a/path.txt",
			data:     []byte("this is a test!this is a test!this is a test!this is a test!this is a test!this is a test!x"),
		},
		{
			name:     "reused path",
			filePath: "this/is/a/path.txt",
			data:     []byte("this is a test!this is a test!this is a test!this is a test!this is a test!this is a test!!this is a test!"),
		},
	}

	for _, fileOperation := range fileOperations {
		t.Run(fileOperation.name, func(t *testing.T) {
			createChangeResp, err := client.CreateChange(context.Background(), &pb.CreateChangeRequest{
				ProjectName: projectName,
			})
			require.NoError(t, err)
			changeId := createChangeResp.GetChangeId()

			currData := fileOperation.data
			err = UploadFile(ctx, client, projectId, changeId, fileOperation.filePath, bytes.NewReader(currData))
			require.NoError(t, err)

			_, err = client.CommitChange(ctx, &pb.CommitChangeRequest{
				ChangeId:  changeId,
				ProjectId: projectId,
			})
			require.NoError(t, err)

			result := new(bytes.Buffer)
			err = DownloadFile(ctx, client, projectId, changeId, fileOperation.filePath, bytes.NewReader(currData), result)
			require.NoError(t, err)

			resultBytes, err := io.ReadAll(result)
			require.NoError(t, err)

			require.Equal(t, currData, resultBytes)
		})
	}

	closeClient()
}
