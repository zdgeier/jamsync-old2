package server

import (
	"bytes"
	"context"
	"database/sql"
	"io"
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
	"google.golang.org/protobuf/proto"
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

func getFileListDiff(ctx context.Context, client pb.JamsyncAPIClient, fileMetadata *pb.FileMetadata, projectId uint64, changeId uint64) (*pb.FileMetadataDiff, error) {
	fileMetadataData, err := proto.Marshal(fileMetadata)
	if err != nil {
		return nil, err
	}
	fileMetadataReader := bytes.NewReader(fileMetadataData)

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	blockHashes := make([]*pb.BlockHash, 0)
	err = rs.CreateSignature(fileMetadataReader, func(bl rsync.BlockHash) error {
		blockHashes = append(blockHashes, &pb.BlockHash{
			Index:      bl.Index,
			StrongHash: bl.StrongHash,
			WeakHash:   bl.WeakHash,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	readFileClient, err := client.ReadFile(ctx, &pb.ReadFileRequest{
		ProjectId:   projectId,
		ChangeId:    changeId,
		PathHash:    pathToHash(".jamsyncfilelist"),
		ModTime:     timestamppb.Now(),
		BlockHashes: blockHashes,
	})
	if err != nil {
		return nil, err
	}

	ops := make(chan rsync.Operation)
	go func() {
		for {
			in, err := readFileClient.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}

			ops <- pbOperationToRsync(in)
		}
		close(ops)
	}()

	result := new(bytes.Buffer)
	fileMetadataReader.Reset(fileMetadataData)
	err = rs.ApplyDelta(result, fileMetadataReader, ops)
	if err != nil {
		return nil, err
	}

	remoteFileMetadata := &pb.FileMetadata{}
	err = proto.Unmarshal(result.Bytes(), remoteFileMetadata)
	if err != nil {
		return nil, err
	}

	fileMetadataDiff := make(map[string]*pb.FileMetadataDiff_FileDiff, len(fileMetadata.GetFiles()))
	for filePath, file := range fileMetadata.GetFiles() {
		fileMetadataDiff[filePath] = &pb.FileMetadataDiff_FileDiff{
			Type: pb.FileMetadataDiff_NoOp,
			File: file,
		}
	}

	for remoteFilePath, remoteFile := range remoteFileMetadata.GetFiles() {
		diffType := pb.FileMetadataDiff_NoOp
		diffFile, found := fileMetadataDiff[remoteFilePath]
		if found && remoteFile.Hash != fileMetadata.GetFiles()[remoteFilePath].GetHash() {
			diffType = pb.FileMetadataDiff_Update
		} else {
			diffType = pb.FileMetadataDiff_Create
		}

		fileMetadataDiff[remoteFilePath] = &pb.FileMetadataDiff_FileDiff{
			Type: diffType,
			File: diffFile.File,
		}
	}

	return &pb.FileMetadataDiff{
		Diffs: fileMetadataDiff,
	}, err
}

func uploadDiff(ctx context.Context, client pb.JamsyncAPIClient, fileMetadata *pb.FileMetadata, fileData map[string][]byte, projectId uint64, changeId uint64) error {
	diff, err := getFileListDiff(ctx, client, fileMetadata, projectId, changeId)
	if err != nil {
		return err
	}

	for filePath, fileDiff := range diff.GetDiffs() {
		if fileDiff.File.Dir || fileDiff.Type == pb.FileMetadataDiff_NoOp {
			continue
		}
		err := uploadFile(ctx, client, projectId, changeId, filePath, bytes.NewReader(fileData[filePath]))
		if err != nil {
			return err
		}
	}

	fileMetadataData, err := proto.Marshal(fileMetadata)
	if err != nil {
		return err
	}
	err = uploadFile(ctx, client, projectId, changeId, ".jamsyncfilemetadata", bytes.NewReader(fileMetadataData))
	if err != nil {
		return err
	}

	return nil
}

func uploadFile(ctx context.Context, client pb.JamsyncAPIClient, projectId uint64, changeId uint64, filePath string, sourceReader io.Reader) error {
	blockHashResp, err := client.ReadBlockHashes(ctx, &pb.ReadBlockHashesRequest{
		ProjectId: projectId,
		ChangeId:  changeId,
		PathHash:  pathToHash(filePath),
	})
	if err != nil {
		return err
	}

	opsOut := make(chan *rsync.Operation)
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}
	go func() {
		var blockCt, blockRangeCt, dataCt, bytes int
		defer close(opsOut)
		err := rsDelta.CreateDelta(sourceReader, PbBlockHashesToRsync(blockHashResp.GetBlockHashes()), func(op rsync.Operation) error {
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
		if err != nil {
			panic(err)
		}
	}()

	writeStream, err := client.WriteOperationStream(ctx)
	if err != nil {
		return err
	}

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
			ProjectId:     projectId,
			ChangeId:      changeId,
			PathHash:      pathToHash(filePath),
			Type:          opPbType,
			BlockIndex:    op.BlockIndex,
			BlockIndexEnd: op.BlockIndexEnd,
			Data:          op.Data,
		})
		if err != nil {
			return err
		}
	}
	return nil
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

		err = uploadDiff(ctx, client, test.localFiles, test.fileData, addProjectResp.GetProjectId(), createChangeResp.GetChangeId())
		require.NoError(t, err)
	}

}

func pathToHash(path string) uint64 {
	h := xxhash.New()
	h.Write([]byte(path))
	return h.Sum64()
}
