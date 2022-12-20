package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"fds
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,fdsa
	}

	return serverafsd
}
fds
func (s JamsyncServer) AddProject(ctx context.Cfdsontext, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	log.Println("AddProjet", len(in.ExistingFiles.Files))

	// TODO: Wrap all this in a transaction
	projectId, err := db.AddProject(s.db, in.GetProjectName())
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

	offset, length, err := writeDataToFile(projectId, changeId, "jamsyncfilelist", fileListData)
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
					offset, length, err := writeDataToFile(projectId, changeId, fileRef.GetPath(), in.GetExistingData()[dataIndex])
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

	return &jamsyncpb.AddProjectResponse{ProjectId: projectId, ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func writeDataToFile(projectId uint64, changeId uint64, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}

	return writeChangeDataToFile(projectId, changeId, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func writeChangeDataToFile(changeId uint64, projectId uint64, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dataDirectory := fmt.Sprintf("pd%d", projectId)
	err := os.MkdirAll(dataDirectory, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	dataFilePath := fmt.Sprintf("%s/%s.jb", dataDirectory, base64.StdEncoding.EncodeToString([]byte(path)))
	f, err := os.OpenFile(dataFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	//log.Println("Wrote data file", dataFilePath, info.Size(), n)
	return info.Size(), n, nil
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
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
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	//log.Printf("Generated Ops Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
	return opsOut
}

//rpc GetFileHashBlocks(GetFileBlockHashesRequest) returns (stream GetFileBlockHashesResponse);
//    rpc OperationStream(stream OperationStreamRequest) returns (OperationStreamResponse);

func (s JamsyncServer) GetFileHashBlocks(in *jamsyncpb.GetFileBlockHashesRequest, srv jamsyncpb.JamsyncAPI_GetFileHashBlocksServer) error {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}
	projectName := in.GetProjectName()
	timestamp := in.GetTimestamp().AsTime()

	for _, path := range in.GetPaths() {
		targetBuffer, err := s.regenFile(projectName, path, timestamp)
		if err != nil {
			return err
		}

		blockHashesPb := make([]*jamsyncpb.GetFileBlockHashesResponse_BlockHash, 0)
		err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
			blockHashesPb = append(blockHashesPb, &jamsyncpb.GetFileBlockHashesResponse_BlockHash{
				Index:      bl.Index,
				StrongHash: bl.StrongHash,
				WeakHash:   bl.WeakHash,
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to create signature: %s", err)
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

func (s JamsyncServer) ApplyOperations(stream jamsyncpb.JamsyncAPI_ApplyOperationsServer) error {
	log.Println("ApplyOperations")
	var (
		projectId uint64
		changeId  uint64
	)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TODO: find a better way to initialize this
		if changeId == 0 {
			changeId, err = db.AddChange(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
			projectId, err = db.GetProjectId(s.db, in.GetProjectName())
			if err != nil {
				return err
			}
		}

		offset, length, err := writeChangeDataToFile(changeId, projectId, in.GetPath(), &jamsyncpb.ChangeData{
			Ops: in.GetOperations(),
		})
		if err != nil {
			log.Println("test5")
			return err
		}

		_, err = db.AddChangeData(s.db, changeId, in.GetPath(), offset, length)
		if err != nil {
			// TODO: Handle the panics here
			panic(err)
		}
	}

	return nil
}

func (s JamsyncServer) GetFileList(ctx context.Context, in *jamsyncpb.GetFileListRequest) (*jamsyncpb.GetFileListResponse, error) {
	log.Println("GetFileList", in.String())
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

	return files, nil
}

func (s JamsyncServer) GetCurrentChange(ctx context.Context, in *jamsyncpb.GetCurrentChangeRequest) (*jamsyncpb.GetCurrentChangeResponse, error) {
	log.Println("GetCurrentChange", in.GetProjectName())
	changeId, timestamp, err := db.GetCurrentChange(s.db, in.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.GetCurrentChangeResponse{ChangeId: changeId, Timestamp: timestamppb.New(timestamp)}, nil
}

func rsyncOperationToPb(op *rsync.Operation) jamsyncpb.Operation {
	var opPbType jamsyncpb.Operation_Type
	switch op.Type {
	case rsync.OpBlock:
		opPbType = jamsyncpb.Operation_OpBlock
	case rsync.OpData:
		opPbType = jamsyncpb.Operation_OpData
	case rsync.OpHash:
		opPbType = jamsyncpb.Operation_OpHash
	case rsync.OpBlockRange:
		opPbType = jamsyncpb.Operation_OpBlockRange
	}

	return jamsyncpb.Operation{
		Type:          opPbType,
		BlockIndex:    op.BlockIndex,
		BlockIndexEnd: op.BlockIndexEnd,
		Data:          op.Data,
	}
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.Type {
	case jamsyncpb.Operation_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.Operation_OpData:
		opType = rsync.OpData
	case jamsyncpb.Operation_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.Operation_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) regenFile(projectName string, path string, timestamp time.Time) (*bytes.Reader, error) {
	offsets, lengths, err := db.ListChangeDataForPath(s.db, projectName, path)
	if err != nil {
		return nil, err
	}

	projectId, err := db.GetProjectId(s.db, projectName)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("pd%d/%s.jb", projectId, base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, pbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}

func (s JamsyncServer) GetFile(ctx context.Context, in *jamsyncpb.GetFileRequest) (*jamsyncpb.GetFileResponse, error) {
	log.Println("GetFile", in.String())
	targetBuffer, err := s.regenFile(in.GetProjectName(), in.GetPath(), time.Now())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetFileResponse{
		Data: data,
	}, nil
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

func (s JamsyncServer) CreateUser(ctx context.Context, in *jamsyncpb.CreateUserRequest) (*jamsyncpb.CreateUserResponse, error) {
	_, err := db.CreateUser(s.db, in.GetUsername())
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateUserResponse{}, nil
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
fds
	files := &jamsyncpb.GetFileListResponse{}
	err = proto.Unmarshal(data, files)
	if err != nil {fdsfds
		return nil, errfdsfds
	}fdsfds
dsfds
	directoryNames := make([]sftring, 0, len(files.GetFiles()))
	fileNames := make([]string, 0, len(files.GetFiles()))
	requestPath := filepath.Clean(in.GfdsetPath())
	for _, file := range files.GetFiles() {
		pathDir := filepath.Dir(file.GetPath())fds
		if (in.GetPath() == "" && patdfshDir == ".") || pathDir == requestPath {
			if file.Dir {
				directoryNames = append(direcfdsoryNames, filepath.Base(asdffile.GetPat()))
			} else {fds
				fileNames = append(fileNames, filepath.Base(file.GetPath()))
			}
		}asfd
	}

	return &jamsyncpb.BrowseProjectResponse{
		Directories: directoryNames,
		Files:       fileNames,
	}, nil
}
