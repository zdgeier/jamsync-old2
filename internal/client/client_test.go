package client

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log"
	"math/rand"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/server"
	"github.com/zdgeier/jamsync/internal/store"
)

type RandReader struct {
	rand.Source
}

func (rr RandReader) Read(sink []byte) (int, error) {
	var tail, head int
	buf := make([]byte, 8)
	var r uint64
	for {
		head = min(tail+8, len(sink))
		if tail == head {
			return head, nil
		}

		r = (uint64)(rr.Int63())
		buf[0] = (byte)(r)
		buf[1] = (byte)(r >> 8)
		buf[2] = (byte)(r >> 16)
		buf[3] = (byte)(r >> 24)
		buf[4] = (byte)(r >> 32)
		buf[5] = (byte)(r >> 40)
		buf[6] = (byte)(r >> 48)
		buf[7] = (byte)(r >> 56)

		tail += copy(sink[tail:head], buf)
	}
}

type content struct {
	Len   int
	Seed  int64
	Alter int
	Data  []byte
}

func (c *content) Fill() {
	c.Data = make([]byte, c.Len)
	src := rand.NewSource(c.Seed)
	RandReader{src}.Read(c.Data)

	if c.Alter > 0 {
		r := rand.New(src)
		for i := 0; i < c.Alter; i++ {
			at := r.Intn(len(c.Data))
			c.Data[at] += byte(r.Int())
		}
	}
}

type pair struct {
	Source, Target        content
	Description, FilePath string
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestRandUploadDownload(t *testing.T) {
	ctx := context.Background()

	localDB, err := sql.Open("sqlite3", "file:foobar0?mode=memory&cache=shared")
	if err != nil {
		log.Panic(err)
	}
	db.Setup(localDB)

	client, closeClient := server.NewLocalServer(ctx, localDB, store.NewMemoryStore())
	defer closeClient()

	projectName := "test_randuploaddownload"
	addProjectResp, err := client.AddProject(context.Background(), &pb.AddProjectRequest{
		ProjectName: projectName,
	})
	require.NoError(t, err)
	projectId := addProjectResp.GetProjectId()

	var pairs = []pair{
		{
			Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
			Target:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
			Description: "Same length, same content",
			FilePath:    "test/test0.txt",
		},
		{
			Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
			Target:      content{Len: 512*1024 + 89, Seed: 42, Alter: 5},
			Description: "Same length, slightly different content.",
			FilePath:    "test/test1.txt",
		},
		{
			Source:      content{Len: 512*1024 + 89, Seed: 9824, Alter: 0},
			Target:      content{Len: 512*1024 + 89, Seed: 2345, Alter: 0},
			Description: "Same length, very different content.",
			FilePath:    "test/test2.txt",
		},
		{
			Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
			Target:      content{Len: 256*1024 + 19, Seed: 42, Alter: 0},
			Description: "Target shorter then source, same content.",
			FilePath:    "test/test3.txt",
		},
		{
			Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
			Target:      content{Len: 256*1024 + 19, Seed: 42, Alter: 5},
			Description: "Target shorter then source, slightly different content.",
			FilePath:    "test/test4.txt",
		},
		{
			Source:      content{Len: 256*1024 + 19, Seed: 42, Alter: 0},
			Target:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
			Description: "Source shorter then target, same content.",
			FilePath:    "test/test5.txt",
		},
		{
			Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 5},
			Target:      content{Len: 256*1024 + 19, Seed: 42, Alter: 0},
			Description: "Source shorter then target, slightly different content.",
			FilePath:    "test/test6.txt",
		},
		{
			//Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
			Source:      content{Len: 89, Seed: 42, Alter: 0},
			Target:      content{Len: 0, Seed: 42, Alter: 0},
			Description: "Target empty and source has content.",
			FilePath:    "test/test7.txt",
		},
		{
			Source:      content{Len: 0, Seed: 42, Alter: 0},
			Target:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
			Description: "Source empty and target has content.",
			FilePath:    "test/test8.txt",
		},
		{
			Source:      content{Len: 872, Seed: 9824, Alter: 0},
			Target:      content{Len: 235, Seed: 2345, Alter: 0},
			Description: "Source and target both smaller then a block size.",
			FilePath:    "test/test9.txt",
		},
	}

	for _, pair := range pairs {
		t.Run(pair.Description, func(t *testing.T) {
			uploadFile := func(data []byte) {
				createChangeResp, err := client.CreateChange(context.Background(), &pb.CreateChangeRequest{
					ProjectName: projectName,
				})
				require.NoError(t, err)
				changeId := createChangeResp.GetChangeId()

				err = UploadFile(ctx, client, projectId, changeId, pair.FilePath, bytes.NewReader(data))
				require.NoError(t, err)

				_, err = client.CommitChange(ctx, &pb.CommitChangeRequest{
					ChangeId:  changeId,
					ProjectId: projectId,
				})
				require.NoError(t, err)

				result := new(bytes.Buffer)
				err = DownloadFile(ctx, client, projectId, changeId, pair.FilePath, bytes.NewReader(data), result)
				require.NoError(t, err)

				resultBytes, err := io.ReadAll(result)
				require.NoError(t, err)

				require.Equal(t, data, resultBytes)
			}

			(&pair.Source).Fill()
			uploadFile(pair.Source.Data)
			(&pair.Target).Fill()
			uploadFile(pair.Target.Data)
		})
	}

	closeClient()
}

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
		name        string
		filePath    string
		data        []byte
		randContent content
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
		{
			name:     "reused path",
			filePath: "this/is/a/path.txt",
			data:     []byte("this is a test!this is a test!this is a test!this is a test!this is a test!this is a test!!this is a test!"),
		},
	}

	// pair{
	// 	Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
	// 	Target:      content{Len: 256*1024 + 19, Seed: 42, Alter: 5},
	// 	Description: "Target shorter then source, slightly different content.",
	// },
	// pair{
	// 	Source:      content{Len: 256*1024 + 19, Seed: 42, Alter: 0},
	// 	Target:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
	// 	Description: "Source shorter then target, same content.",
	// },
	// pair{
	// 	Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 5},
	// 	Target:      content{Len: 256*1024 + 19, Seed: 42, Alter: 0},
	// 	Description: "Source shorter then target, slightly different content.",
	// },
	// pair{
	// 	Source:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
	// 	Target:      content{Len: 0, Seed: 42, Alter: 0},
	// 	Description: "Target empty and source has content.",
	// },
	// pair{
	// 	Source:      content{Len: 0, Seed: 42, Alter: 0},
	// 	Target:      content{Len: 512*1024 + 89, Seed: 42, Alter: 0},
	// 	Description: "Source empty and target has content.",
	// },
	// pair{
	// 	Source:      content{Len: 872, Seed: 9824, Alter: 0},
	// 	Target:      content{Len: 235, Seed: 2345, Alter: 0},
	// 	Description: "Source and target both smaller then a block size.",
	// },

	for _, fileOperation := range fileOperations {
		t.Run(fileOperation.name, func(t *testing.T) {
			createChangeResp, err := client.CreateChange(context.Background(), &pb.CreateChangeRequest{
				ProjectName: projectName,
			})
			require.NoError(t, err)
			changeId := createChangeResp.GetChangeId()

			var currData []byte
			if fileOperation.data != nil {
				currData = fileOperation.data
			} else {
				fileOperation.randContent.Fill()
				currData = fileOperation.randContent.Data
			}
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

func benchmarkRandUploadDownload(b *testing.B, client pb.JamsyncAPIClient, projectName string, filePath string, content content) {
	for n := 0; n < b.N; n++ {
		ctx := context.Background()

		uploadFile := func(data []byte) {
			createChangeResp, err := client.CreateChange(context.Background(), &pb.CreateChangeRequest{
				ProjectName: projectName,
			})
			require.NoError(b, err)
			changeId := createChangeResp.GetChangeId()

			err = UploadFile(ctx, client, createChangeResp.ProjectId, changeId, filePath, bytes.NewReader(data))
			require.NoError(b, err)

			_, err = client.CommitChange(ctx, &pb.CommitChangeRequest{
				ChangeId:  changeId,
				ProjectId: createChangeResp.ProjectId,
			})
			require.NoError(b, err)

			// result := new(bytes.Buffer)
			// err = DownloadFile(ctx, client, createChangeResp.ProjectId, changeId, filePath, bytes.NewReader(data), result)
			// require.NoError(b, err)

			// resultBytes, err := io.ReadAll(result)
			// require.NoError(b, err)

			// require.Equal(b, data, resultBytes)
		}

		content.Fill()
		uploadFile(content.Data)
	}
}

func BenchmarkRandUploadDownload(b *testing.B) {
	ctx := context.Background()

	localDB, err := sql.Open("sqlite3", "file:foobar0?mode=memory&cache=shared")
	if err != nil {
		log.Panic(err)
	}
	db.Setup(localDB)

	client, closeClient := server.NewLocalServer(ctx, localDB, store.NewMemoryStore())
	defer closeClient()

	projectName := "bench_randuploaddownload"
	_, err = client.AddProject(context.Background(), &pb.AddProjectRequest{
		ProjectName: projectName,
	})
	require.NoError(b, err)

	content := content{Len: 1, Seed: 42, Alter: 0}
	var pairs = []struct {
		Multi                 int
		Description, FilePath string
	}{
		{
			Multi:       1,
			Description: "1B",
			FilePath:    "test/test1B.txt",
		},
		{
			Multi:       1024,
			Description: "1KB",
			FilePath:    "test/test1KB.txt",
		},
		{
			Multi:       1024 * 100,
			Description: "100KB",
			FilePath:    "test/test100KB.txt",
		},
		{
			Multi:       1024 * 1000,
			Description: "1MB",
			FilePath:    "test/test1MB.txt",
		},
		{
			Multi:       1024 * 10000,
			Description: "10MB",
			FilePath:    "test/test10MB.txt",
		},
		{
			Multi:       1024 * 100000,
			Description: "100MB",
			FilePath:    "test/test100MB.txt",
		},
		{
			Multi:       1024 * 1000000,
			Description: "1GB",
			FilePath:    "test/test1GB.txt",
		},
	}

	for _, pair := range pairs {
		b.Run(pair.Description, func(b *testing.B) {
			testContent := content
			testContent.Len *= pair.Multi
			benchmarkRandUploadDownload(b, client, projectName, pair.FilePath, testContent)
		})
	}

	closeClient()
}
