package server

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"net"
	"testing"
	"time"

	"github.com/cespare/xxhash"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"github.com/zdgeier/jamsync/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func server(ctx context.Context, db *sql.DB, store store.Store) (pb.JamsyncAPIClient, func()) {
	buffer := 101024 * 1024
	lis := bufconn.Listen(buffer)

	baseServer := grpc.NewServer()
	pb.RegisterJamsyncAPIServer(baseServer, NewServer(db, store))
	go func() {
		if err := baseServer.Serve(lis); err != nil {
			log.Printf("error serving server: %v", err)
		}
	}()

	conn, err := grpc.DialContext(ctx, "",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("error connecting to server: %v", err)
	}

	closer := func() {
		err := lis.Close()
		if err != nil {
			log.Printf("error closing listener: %v", err)
		}
		baseServer.Stop()
	}

	client := pb.NewJamsyncAPIClient(conn)

	return client, closer
}

func uploadFiles() {
	for filePath, file := range test.localFiles.Files {
		if file.Dir {
			continue
		}
		blockHashResp, err := client.ReadBlockHashes(ctx, &pb.ReadBlockHashesRequest{
			ProjectId: addProjectResp.GetProjectId(),
			ChangeId:  createChangeResp.ChangeId,
			PathHash:  pathToHash(filePath),
		})
		require.NoError(t, err)

		blockHashes := make([]rsync.BlockHash, 0)
		for _, pbBlockHash := range blockHashResp.GetBlockHashes() {
			blockHashes = append(blockHashes, rsync.BlockHash{
				Index:      pbBlockHash.GetIndex(),
				StrongHash: pbBlockHash.GetStrongHash(),
				WeakHash:   pbBlockHash.GetWeakHash(),
			})
		}

		opsOut := make(chan *rsync.Operation)
		rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}
		go func() {
			sourceBuffer := bytes.NewReader(test.fileData[filePath])
			var blockCt, blockRangeCt, dataCt, bytes int
			defer close(opsOut)
			err := rsDelta.CreateDelta(sourceBuffer, blockHashes, func(op rsync.Operation) error {
				switch op.Type {
				case rsync.OpBlockRange:
					blockRangeCt++
				case rsync.OpBlock:
					blockCt++
				case rsync.OpData:
					// Copy data buffer so it may be reused in internal buffer.
					b := make([]byte, len(op.Data))
					copy(b, op.Data)
					op.Data = b
					dataCt++
					bytes += len(op.Data)
				}
				opsOut <- &op
				return nil
			})
			require.NoError(t, err)
		}()

		writeStream, err := client.WriteOperationStream(ctx)
		require.NoError(t, err)

		for op := range opsOut {
			var opPbType pb.Operation_Type
			switch op.Type {
			case rsync.OpBlock:
				opPbType = pb.Operation_OpBlock
			case rsync.OpData:
				opPbType = pb.Operation_OpData
			case rsync.OpHash:
				opPbType = pb.Operation_OpHash
			case rsync.OpBlockRange:
				opPbType = pb.Operation_OpBlockRange
			}

			err = writeStream.Send(&pb.Operation{
				ProjectId:     addProjectResp.GetProjectId(),
				ChangeId:      createChangeResp.GetChangeId(),
				PathHash:      pathToHash(filePath),
				Type:          opPbType,
				BlockIndex:    op.BlockIndex,
				BlockIndexEnd: op.BlockIndexEnd,
				Data:          op.Data,
			})
			require.NoError(t, err)
		}
	}
}

func TestClient(t *testing.T) {
	ctx := context.Background()

	localDB, err := sql.Open("sqlite3", "file:foobar0?mode=memory&cache=shared")
	if err != nil {
		log.Panic(err)
	}
	db.Setup(localDB)

	client, closeClient := server(ctx, localDB, store.NewMemoryStore())
	defer closeClient()

	rows, err := localDB.Query("SELECT name FROM sqlite_schema WHERE type='table' ORDER BY name;")
	require.NoError(t, err)
	defer rows.Close()

	tests := []struct {
		projectName string
		localFiles  *pb.FileMetadata
		fileData    map[string][]byte
	}{
		{
			projectName: "test",
			localFiles: &pb.FileMetadata{
				Files: map[string]*pb.File{
					"test.txt": {
						ModTime: timestamppb.New(time.UnixMilli(100)),
					},
					"testdir": {
						ModTime: timestamppb.New(time.UnixMilli(200)),
						Dir:     true,
					},
					"testdir/test2.txt": {
						ModTime: timestamppb.New(time.UnixMilli(200)),
					},
				},
			},
			fileData: map[string][]byte{
				"test.txt":  []byte("this is a test!"),
				"test2.txt": []byte("this is another test!"),
			},
		},
		{
			projectName: "emptytest",
			localFiles: &pb.FileMetadata{
				Files: map[string]*pb.File{},
			},
			fileData: map[string][]byte{},
		},
	}

	for _, test := range tests {
		addProjectResp, err := client.AddProject(context.Background(), &pb.AddProjectRequest{
			ProjectName: test.projectName,
		})
		require.NoError(t, err)
		createChangeResp, err := client.CreateChange(context.Background(), &pb.CreateChangeRequest{
			ProjectName: test.projectName,
		})
		require.NoError(t, err)

	}
}

func pathToHash(path string) uint64 {
	h := xxhash.New()
	h.Write([]byte(path))
	return h.Sum64()
}
