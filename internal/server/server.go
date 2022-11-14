package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/rsync"
	"google.golang.org/protobuf/proto"
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
		projectsPb[i] = &jamsyncpb.ListProjectsResponse_Project{Name: projects[i].Name, OwnerUserId: projects[i].OwnerId, Id: projects[i].Id}
	}

	return &jamsyncpb.ListProjectsResponse{Projects: projectsPb}, nil
}

func (s JamsyncServer) AddUser(ctx context.Context, in *jamsyncpb.AddUserRequest) (*jamsyncpb.AddUserResponse, error) {
	id, err := db.AddUser(s.db, in.Username)
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.AddUserResponse{UserId: id}, nil
}

func (s JamsyncServer) GetUser(ctx context.Context, in *jamsyncpb.GetUserRequest) (*jamsyncpb.GetUserResponse, error) {
	id, err := db.GetUser(s.db, in.GetUserId())
	if err != nil {
		return nil, err
	}

	return &jamsyncpb.GetUserResponse{Username: id}, nil
}

func (s JamsyncServer) ListUsers(ctx context.Context, in *jamsyncpb.ListUsersRequest) (*jamsyncpb.ListUsersResponse, error) {
	users, err := db.ListUsers(s.db)
	if err != nil {
		return nil, err
	}

	usersPb := make([]*jamsyncpb.ListUsersResponse_User, len(users))
	for i := range usersPb {
		usersPb[i] = &jamsyncpb.ListUsersResponse_User{Username: users[i].Username, UserId: users[i].Id}
	}

	return &jamsyncpb.ListUsersResponse{Users: usersPb}, nil
}

func (s JamsyncServer) UpdateStream(stream jamsyncpb.JamsyncAPI_UpdateStreamServer) error {
	var (
		userId, branchId, projectId uint64
		path                        string
		change                      jamsyncpb.Change
		err                         error
	)
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
				break
			}

			// TODO: Find a better way to encode this information
			projectId = in.GetProjectId()
			branchId = in.GetBranchId()
			userId = in.GetUserId()
			path = in.GetPath()

			change.Ops = append(change.Ops, in.Operation)
		}
	}()

	var changeId uint64
	if path == "" {
		changeId, err = db.AddManifestChange(s.db, branchId, userId, projectId)
		if err != nil {
			log.Fatal(err)
			return err
		}
	} else {
		changeId, err = db.AddChange(s.db, branchId, userId, projectId)
		if err != nil {
			log.Fatal(err)
			return err
		}
	}

	data, err := proto.Marshal(&change)
	if err != nil {
		return err
	}

	if path == "" {
		err = os.WriteFile(fmt.Sprintf("%d.mjb", changeId), data, 0644)
		if err != nil {
			return err
		}
	} else {
		err = os.WriteFile(fmt.Sprintf("%s.%d.jb", base64.StdEncoding.EncodeToString([]byte(path)), changeId), data, 0644)
		if err != nil {
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

	return rsync.Operation{
		Type:          opType,
		BlockIndex:    op.GetBlockIndex(),
		BlockIndexEnd: op.GetBlockIndexEnd(),
		Data:          op.GetData(),
	}
}

func (s JamsyncServer) GetBlockHashes(ctx context.Context, in *jamsyncpb.GetBlockHashesRequest) (*jamsyncpb.GetBlockHashesResponse, error) {
	rs := &rsync.RSync{}

	ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	file, err := s.RegenFile(ctx2, &jamsyncpb.RegenFileRequest{
		ProjectId: in.GetProjectId(),
		BranchId:  in.GetProjectId(),
		Path:      in.GetPath(),
	})
	if err != nil {
		return nil, err
	}
	log.Println("test2")
	targetBuffer := bytes.NewReader(file.GetData())

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
		log.Fatalf("Failed to create signature: %s", err)
	}

	return &jamsyncpb.GetBlockHashesResponse{BlockHashes: blockHashesPb}, nil
}

func (s JamsyncServer) RegenFile(ctx context.Context, in *jamsyncpb.RegenFileRequest) (*jamsyncpb.RegenFileResponse, error) {
	changes := make([]*jamsyncpb.Change, 0)
	fmt.Println("1")
	if in.GetPath() == "" {
		ids, err := db.ListManifestChanges(s.db, in.GetBranchId(), in.GetProjectId())
		if err != nil {
			return nil, err
		}

		fmt.Println("10")
		for _, id := range ids {
			file, err := os.ReadFile(fmt.Sprintf("%d.mjb", id))
			if err != nil {
				return nil, err
			}
			change := &jamsyncpb.Change{}
			err = proto.Unmarshal(file, change)
			if err != nil {
				return nil, err
			}
			changes = append(changes, change)
		}
	} else {
		fmt.Println("101")
		ids, err := db.ListChanges(s.db, in.GetBranchId(), in.GetProjectId())
		if err != nil {
			fmt.Println("asdf2")
			return nil, err
		}
		//ids := []uint64{}

		fmt.Println("2")
		for _, id := range ids {
			file, err := os.ReadFile(fmt.Sprintf("%s.%d.jb", base64.StdEncoding.EncodeToString([]byte(in.GetPath())), id))
			if err != nil {
				return nil, err
			}
			fmt.Println("4")
			change := &jamsyncpb.Change{}
			err = proto.Unmarshal(file, change)
			if err != nil {
				return nil, err
			}
			changes = append(changes, change)
		}
		fmt.Println("3")
	}
	fmt.Println(changes)

	targetBuffer := &bytes.Reader{}
	for _, change := range changes {
		opsOut := make(chan rsync.Operation)
		go func() {
			for _, op := range change.GetOps() {
				opsOut <- pbOperationToRsync(op)
			}
			close(opsOut)
		}()

		result := new(bytes.Buffer)
		rs := rsync.RSync{}
		err := rs.ApplyDelta(result, targetBuffer, opsOut)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(targetBuffer)
		if err != nil {
			return nil, err
		}
		targetBuffer = bytes.NewReader([]byte(data))
	}

	data, err := io.ReadAll(targetBuffer)
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.RegenFileResponse{
		Data: data,
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
