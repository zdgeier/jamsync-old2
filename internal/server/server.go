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
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/pierrec/lz4/v4"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
}

func (s JamsyncServer) AddProject(ctx context.Context, in *jamsyncpb.AddProjectRequest) (*jamsyncpb.AddProjectResponse, error) {
	res, err := db.AddProject(s.db, in.GetName(), in.GetOwner())
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.AddProjectResponse{ProjectId: res}, nil
}

func (s JamsyncServer) GetProject(ctx context.Context, in *jamsyncpb.GetProjectRequest) (*jamsyncpb.GetProjectResponse, error) {
	name, res, err := db.GetProject(s.db, in.GetProjectId())
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetProjectResponse{Name: name, Owner: res}, nil
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

func (s JamsyncServer) UpdateStream(stream jamsyncpb.JamsyncAPI_UpdateStreamServer) error {
	var currChange *jamsyncpb.Change
	changes := make(chan *jamsyncpb.Change)
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				// TODO: this triggers with context cancelled for some reason
				log.Println(err)
				return
			}

			if in.GetPathData().GetPath() != "" {
				if currChange != nil {
					changes <- currChange
				}

				currChange = &jamsyncpb.Change{
					ProjectId: in.GetProjectId(),
					BranchId:  in.GetBranchId(),
					PathData: &jamsyncpb.PathData{
						Path: in.GetPathData().GetPath(),
						Hash: in.GetPathData().GetHash(),
					},
				}
			}
			currChange.Ops = append(currChange.Ops, in.Operation)
		}
		changes <- currChange
		close(changes)
	}()

	for change := range changes {
		f, err := os.OpenFile(fmt.Sprintf("data/%s.jb", base64.StdEncoding.EncodeToString([]byte(change.GetPathData().GetPath()))), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}

		info, err := f.Stat()
		if err != nil {
			return err
		}

		data, err := proto.Marshal(change)
		if err != nil {
			return err
		}

		n, err := f.Write(data)
		if err != nil {
			log.Panic(err)
			return err
		}

		_, err = db.AddChange(s.db, change.GetBranchId(), change.GetProjectId(), info.Size(), int64(n))
		if err != nil {
			log.Panic(err)
			return err
		}
	}

	return nil
}

func pbOperationToRsync(op *jamsyncpb.Operation) rsync.Operation {
	var opType rsync.OpType
	switch op.OpType {
	case jamsyncpb.OpType_OpBlock:
		opType = rsync.OpBlock
	case jamsyncpb.OpType_OpData:
		opType = rsync.OpData
	case jamsyncpb.OpType_OpHash:
		opType = rsync.OpHash
	case jamsyncpb.OpType_OpBlockRange:
		opType = rsync.OpBlockRange
	}

	out := new(bytes.Buffer)
	zr := lz4.NewReader(bytes.NewReader(op.GetData()))
	_, err := io.Copy(out, zr)
	if err != nil {
		log.Panic(err)
	}

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          out.Bytes(),
	}
}

func (s JamsyncServer) GetBlockHashes(ctx context.Context, in *jamsyncpb.GetBlockHashesRequest) (*jamsyncpb.GetBlockHashesResponse, error) {
	rs := &rsync.RSync{UniqueHasher: xxhash.New()}

	targetBuffer, err := s.regenFile(in.GetPath(), in.GetBranchId(), in.GetProjectId(), in.GetTimestamp().AsTime())
	if err != nil {
		return nil, err
	}

	blockHashesPb := make([]*jamsyncpb.GetBlockHashesResponse_BlockHash, 0)
	err = rs.CreateSignature(targetBuffer, func(bl rsync.BlockHash) error {
		blockHashesPb = append(blockHashesPb, &jamsyncpb.GetBlockHashesResponse_BlockHash{
			Index:      bl.Index,
			StrongHash: bl.StrongHash,
			WeakHash:   bl.WeakHash,
		})
		return nil
	})
	if err != nil {
		log.Panicf("Failed to create signature: %s", err)
	}

	return &jamsyncpb.GetBlockHashesResponse{BlockHashes: blockHashesPb}, nil
}

func (s JamsyncServer) regenFile(path string, branchId uint64, projectId uint64, timestamp time.Time) (*bytes.Reader, error) {
	changes := make([]*jamsyncpb.Change, 0)
	_, _, offsets, lengths, err := db.ListChanges(s.db, branchId, projectId, timestamp.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}

	f, err := os.Open(fmt.Sprintf("data/%s.jb", base64.StdEncoding.EncodeToString([]byte(path))))
	if errors.Is(err, os.ErrNotExist) {
		return &bytes.Reader{}, nil
	}
	if err != nil {
		return nil, err
	}

	for i := range lengths {
		changeFile := make([]byte, lengths[i])
		n, err := f.ReadAt(changeFile, int64(offsets[i]))
		if err != nil {
			return nil, err
		}
		if n != int(lengths[i]) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.Change{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}

	changeOps := make([][]rsync.Operation, 0, len(changes))
	for _, change := range changes {
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
		targetBuffer = bytes.NewReader(result.Bytes())
		result.Reset()
	}

	fmt.Println(targetBuffer.Len())
	return targetBuffer, nil
}

func (s JamsyncServer) RegenFile(ctx context.Context, in *jamsyncpb.RegenFileRequest) (*jamsyncpb.RegenFileResponse, error) {
	targetBuffer, err := s.regenFile(in.GetPath(), in.GetBranchId(), in.GetProjectId(), in.GetTimestamp().AsTime())
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.RegenFileResponse{
		Data: string(data),
	}, nil
}

func (s JamsyncServer) ListChanges(ctx context.Context, in *jamsyncpb.ListChangesRequest) (*jamsyncpb.ListChangesResponse, error) {
	ids, times, _, _, err := db.ListChanges(s.db, in.GetBranchId(), in.GetProjectId(), in.GetTimestamp().AsTime())
	if err != nil {
		return nil, err
	}

	changeRows := make([]*jamsyncpb.ListChangesResponse_ChangeRow, 0)
	for i := range ids {
		changeRows = append(changeRows, &jamsyncpb.ListChangesResponse_ChangeRow{
			Id:        ids[i],
			Timestamp: timestamppb.New(times[i]),
		})
	}

	return &jamsyncpb.ListChangesResponse{
		ChangeRows: changeRows,
	}, nil
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,
	}

	return server
}

func (s JamsyncServer) GenTestData() {
	id, _ := db.AddUser(s.db, "zdgeier")
	db.AddUser(s.db, "testuser1")
	db.AddUser(s.db, "testuser2")
	db.AddUser(s.db, "testuser3")
	db.AddUser(s.db, "testuser4")
	db.AddProject(s.db, "TestProject", id)
	db.AddProject(s.db, "Jamsync", id)
	db.AddProject(s.db, "JamsyncOpen", id)
}
