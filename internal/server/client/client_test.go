package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/jamenv"
	"github.com/zdgeier/jamsync/internal/server/clientauth"
	"github.com/zdgeier/jamsync/internal/server/server"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
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

var serverRunning = false

func setup() (pb.JamsyncAPIClient, func(), error) {
	if !serverRunning {
		if jamenv.Env() == jamenv.Local {
			err := os.RemoveAll("jb/")
			if err != nil {
				log.Fatal(err)
			}
			err = os.RemoveAll("jamsync.db")
			if err != nil {
				log.Fatal(err)
			}
		}
		_, err := server.New()
		if err != nil && !strings.Contains(err.Error(), "bind: address already in use") {
			return nil, nil, err
		}
		serverRunning = true
	}

	accessToken, err := clientauth.InitConfig()
	if err != nil {
		return nil, nil, err
	}

	return server.Connect(&oauth2.Token{AccessToken: accessToken})
}

func TestClient_UploadDownload(t *testing.T) {
	ctx := context.Background()

	apiClient, closer, err := setup()
	require.NoError(t, err)
	defer closer()

	projectName := "test"

	addProjectResp, err := apiClient.AddProject(context.Background(), &pb.AddProjectRequest{
		ProjectName: projectName,
	})
	require.NoError(t, err)

	client := NewClient(apiClient, addProjectResp.ProjectId, 1)

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

	for _, fileOperation := range fileOperations {
		t.Run(fileOperation.name, func(t *testing.T) {
			err := client.CreateChange()
			require.NoError(t, err)

			var currData []byte
			if fileOperation.data != nil {
				currData = fileOperation.data
			} else {
				fileOperation.randContent.Fill()
				currData = fileOperation.randContent.Data
			}
			err = client.UploadFile(ctx, fileOperation.filePath, bytes.NewReader(currData))
			require.NoError(t, err)

			client.CommitChange()
			require.NoError(t, err)

			result := new(bytes.Buffer)
			err = client.DownloadFile(ctx, fileOperation.filePath, bytes.NewReader(currData), result)
			require.NoError(t, err)

			require.Equal(t, currData, result.Bytes())
		})
	}
}

func TestClient_RandUploadDownload(t *testing.T) {
	ctx := context.Background()

	apiClient, closer, err := setup()
	require.NoError(t, err)
	defer closer()

	projectName := "test_randuploaddownload"
	addProjectResp, err := apiClient.AddProject(context.Background(), &pb.AddProjectRequest{
		ProjectName: projectName,
	})
	require.NoError(t, err)
	projectId := addProjectResp.GetProjectId()

	client := NewClient(apiClient, projectId, 1)

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
				client.CreateChange()
				require.NoError(t, err)

				err = client.UploadFile(ctx, pair.FilePath, bytes.NewReader(data))
				require.NoError(t, err)

				client.CommitChange()
				require.NoError(t, err)

				result := new(bytes.Buffer)
				err = client.DownloadFile(ctx, pair.FilePath, bytes.NewReader(data), result)
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
}

func benchmarkUpload(b *testing.B, client *Client, projectName string, filePath string, content content) {
	for n := 0; n < b.N; n++ {
		ctx := context.Background()

		uploadFile := func(data []byte) {
			err := client.CreateChange()
			require.NoError(b, err)

			err = client.UploadFile(ctx, filePath+fmt.Sprint(n), bytes.NewReader(data))
			require.NoError(b, err)

			// err = client.CommitChange()
			// require.NoError(b, err)
		}

		content.Fill()
		uploadFile(content.Data)
	}
}

// func benchmarkDownload(b *testing.B, client *Client, projectId uint64, changeId uint64, filePath string, content content) {
// 	for n := 0; n < b.N; n++ {
// 		ctx := context.Background()
//
// 		downloadFile := func(data []byte) {
// 			result := new(bytes.Buffer)
// 			err := client.DownloadFile(ctx, filePath+fmt.Sprint(n), bytes.NewReader(data), result)
// 			require.NoError(b, err)
//
// 			resultBytes, err := io.ReadAll(result)
// 			require.NoError(b, err)
//
// 			require.Equal(b, data, resultBytes)
// 		}
//
// 		content.Fill()
// 		downloadFile(content.Data)
// 	}
// }

func BenchmarkRandUpload(b *testing.B) {
	apiClient, closer, err := setup()
	require.NoError(b, err)
	defer closer()

	projectName := "bench_randupload"

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

	for i, pair := range pairs {
		addProjResp, err := apiClient.AddProject(context.Background(), &pb.AddProjectRequest{
			ProjectName: projectName + fmt.Sprint(i),
		})
		require.NoError(b, err)
		client := NewClient(apiClient, addProjResp.ProjectId, 0)

		b.Run(pair.Description, func(b *testing.B) {
			testContent := content
			testContent.Len *= pair.Multi
			benchmarkUpload(b, client, projectName, pair.FilePath, testContent)
		})

		// b.Run(pair.Description+"DL", func(b *testing.B) {
		// 	testContent := content
		// 	testContent.Len *= pair.Multi
		// 	benchmarkDownload(b, client, addProjResp.GetProjectId(), ^uint64(0), pair.FilePath, testContent)
		// })
	}
}

func TestGetFileListDiff(t *testing.T) {
	ctx := context.Background()
	apiClient, closeClient, err := setup()
	require.NoError(t, err)
	defer closeClient()

	projectName := "test_getfilelistdiff"
	addProjectResp, err := apiClient.AddProject(context.Background(), &pb.AddProjectRequest{
		ProjectName: projectName,
	})
	require.NoError(t, err)

	client := NewClient(apiClient, addProjectResp.ProjectId, 0)

	initMetadata := &pb.FileMetadata{
		Files: map[string]*pb.File{
			"test.txt": {
				ModTime: timestamppb.New(time.UnixMilli(100)),
				Hash:    1234,
			},
			"testdir": {
				ModTime: timestamppb.New(time.UnixMilli(200)),
				Dir:     true,
			},
			"testdir/test2.txt": {
				ModTime: timestamppb.New(time.UnixMilli(200)),
				Hash:    123,
			},
		},
	}

	type args struct {
		ctx          context.Context
		client       *Client
		fileMetadata *pb.FileMetadata
		projectId    uint64
	}
	tests := []struct {
		name    string
		args    args
		want    *pb.FileMetadataDiff
		wantErr bool
	}{
		{
			name: "Simple",
			args: args{
				ctx:          ctx,
				client:       client,
				fileMetadata: initMetadata,
				projectId:    addProjectResp.GetProjectId(),
			},
			want: &pb.FileMetadataDiff{
				Diffs: map[string]*pb.FileMetadataDiff_FileDiff{
					"test.txt": {
						Type: pb.FileMetadataDiff_Create,
						File: &pb.File{
							ModTime: timestamppb.New(time.UnixMilli(100)),
							Dir:     false,
							Hash:    1234,
						},
					},
					"testdir": {
						Type: pb.FileMetadataDiff_Create,
						File: &pb.File{
							ModTime: timestamppb.New(time.UnixMilli(200)),
							Dir:     true,
						},
					},
					"testdir/test2.txt": {
						Type: pb.FileMetadataDiff_Create,
						File: &pb.File{
							ModTime: timestamppb.New(time.UnixMilli(200)),
							Dir:     false,
							Hash:    123,
						},
					},
				},
			},
		},
		{
			name: "Noop after upload",
			args: args{
				ctx:          ctx,
				client:       client,
				fileMetadata: initMetadata,
				projectId:    addProjectResp.GetProjectId(),
			},
			want: &pb.FileMetadataDiff{
				Diffs: map[string]*pb.FileMetadataDiff_FileDiff{
					"test.txt": {
						Type: pb.FileMetadataDiff_NoOp,
					},
					"testdir": {
						Type: pb.FileMetadataDiff_NoOp,
					},
					"testdir/test2.txt": {
						Type: pb.FileMetadataDiff_NoOp,
					},
				},
			},
		},
		{
			name: "Changed",
			args: args{
				ctx:    ctx,
				client: client,
				fileMetadata: &pb.FileMetadata{
					Files: map[string]*pb.File{
						"test2.txt": {
							ModTime: timestamppb.New(time.UnixMilli(100)),
							Hash:    1234,
						},
						"testdir": {
							ModTime: timestamppb.New(time.UnixMilli(200)),
							Dir:     true,
						},
						"testdir2": {
							ModTime: timestamppb.New(time.UnixMilli(200)),
							Dir:     true,
						},
						"testdir/test2.txt": {
							ModTime: timestamppb.New(time.UnixMilli(300)),
							Hash:    12345,
						},
					},
				},
				projectId: addProjectResp.GetProjectId(),
			},
			want: &pb.FileMetadataDiff{
				Diffs: map[string]*pb.FileMetadataDiff_FileDiff{
					"test.txt": {
						Type: pb.FileMetadataDiff_Delete,
					},
					"test2.txt": {
						Type: pb.FileMetadataDiff_Create,
						File: &pb.File{
							ModTime: timestamppb.New(time.UnixMilli(100)),
							Dir:     false,
							Hash:    1234,
						},
					},
					"testdir": {
						Type: pb.FileMetadataDiff_NoOp,
					},
					"testdir2": {
						Type: pb.FileMetadataDiff_Create,
						File: &pb.File{
							ModTime: timestamppb.New(time.UnixMilli(200)),
							Dir:     true,
						},
					},
					"testdir/test2.txt": {
						Type: pb.FileMetadataDiff_Update,
						File: &pb.File{
							ModTime: timestamppb.New(time.UnixMilli(300)),
							Dir:     false,
							Hash:    12345,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.args.client.CreateChange()
			require.NoError(t, err)

			got, err := client.DiffLocalToRemote(tt.args.ctx, tt.args.fileMetadata)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetFileListDiff() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !proto.Equal(got, tt.want) {
				t.Errorf("GetFileListDiff() = %v, want %v", got, tt.want)
			}

			err = client.CreateChange()
			require.NoError(t, err)

			metadataBytes, err := proto.Marshal(tt.args.fileMetadata)
			require.NoError(t, err)

			err = client.UploadFile(ctx, ".jamsyncfilelist", bytes.NewReader(metadataBytes))
			require.NoError(t, err)

			err = client.CommitChange()
			require.NoError(t, err)
		})
	}
}
