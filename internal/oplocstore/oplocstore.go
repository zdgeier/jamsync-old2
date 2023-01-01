package oplocstore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/jamenv"
	"google.golang.org/protobuf/proto"
)

type OpLocStore interface {
	InsertOperationLocations(opLocs *pb.OperationLocations) error
	ListOperationLocations(projectId uint64, pathHash uint64, changeId uint64) (*pb.OperationLocations, error)
}

func New() OpLocStore {
	var opLocStore OpLocStore
	switch jamenv.Env() {
	case jamenv.Prod, jamenv.Dev, jamenv.Local:
		opLocStore = NewLocalOpLocStore("jb")
	case jamenv.Memory:
		opLocStore = NewMemoryOpLocStore()
	}
	return opLocStore
}

type LocalOpLocStore struct {
	directory     string
	openFileCache map[string]*os.File
}

func (s LocalOpLocStore) opLocDirectory(projectId uint64, changeId uint64) string {
	return fmt.Sprintf("%s/%d/oplocs/%d", s.directory, projectId, changeId)
}
func (s LocalOpLocStore) filePath(projectId uint64, changeId uint64, pathHash uint64) string {
	return fmt.Sprintf("%s/%d.locs", s.opLocDirectory(projectId, changeId), pathHash)
}

func NewLocalOpLocStore(directory string) LocalOpLocStore {
	return LocalOpLocStore{
		directory: directory,
		//openFileCache: make(map[string]*os.File),
	}
}

func (s LocalOpLocStore) InsertOperationLocations(opLocs *pb.OperationLocations) error {
	err := os.MkdirAll(s.opLocDirectory(opLocs.GetProjectId(), opLocs.GetChangeId()), os.ModePerm)
	if err != nil {
		return err
	}
	filePath := s.filePath(opLocs.GetProjectId(), opLocs.GetChangeId(), opLocs.GetPathHash())
	currFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	bytes, err := proto.Marshal(opLocs)
	if err != nil {
		return err
	}
	_, err = currFile.Write(bytes)
	return err
}
func (s LocalOpLocStore) ListOperationLocations(projectId uint64, pathHash uint64, changeId uint64) (opLocs *pb.OperationLocations, err error) {
	filePath := s.filePath(projectId, changeId, pathHash)

	_, err = os.Stat(filePath)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	currFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	currFile.Seek(0, 0)
	_, err = io.Copy(buf, currFile)
	if err != nil {
		return nil, err
	}

	opLocs = &pb.OperationLocations{}
	err = proto.Unmarshal(buf.Bytes(), opLocs)
	return opLocs, err
}

type MemoryOpLocStore struct {
	opLocs map[uint64]map[uint64]map[uint64]*pb.OperationLocations // projectId:changeId:pathHash -> opLocs
}

func NewMemoryOpLocStore() MemoryOpLocStore {
	return MemoryOpLocStore{
		opLocs: make(map[uint64]map[uint64]map[uint64]*pb.OperationLocations, 0),
	}
}

func (s MemoryOpLocStore) InsertOperationLocations(opLocs *pb.OperationLocations) error {
	if _, ok := s.opLocs[opLocs.ProjectId]; !ok {
		s.opLocs[opLocs.ProjectId] = make(map[uint64]map[uint64]*pb.OperationLocations)
	}
	if _, ok := s.opLocs[opLocs.ProjectId][opLocs.ChangeId]; !ok {
		s.opLocs[opLocs.ProjectId][opLocs.ChangeId] = make(map[uint64]*pb.OperationLocations)
	}
	if _, ok := s.opLocs[opLocs.ProjectId][opLocs.ChangeId][opLocs.PathHash]; !ok {
		s.opLocs[opLocs.ProjectId][opLocs.ChangeId][opLocs.PathHash] = &pb.OperationLocations{}
	}
	s.opLocs[opLocs.ProjectId][opLocs.ChangeId][opLocs.PathHash] = opLocs
	return nil
}

func (s MemoryOpLocStore) ListOperationLocations(projectId uint64, pathHash uint64, changeId uint64) (*pb.OperationLocations, error) {
	if _, ok := s.opLocs[projectId]; !ok {
		s.opLocs[projectId] = make(map[uint64]map[uint64]*pb.OperationLocations)
	}
	if _, ok := s.opLocs[projectId][changeId]; !ok {
		s.opLocs[projectId][changeId] = make(map[uint64]*pb.OperationLocations)
	}
	if _, ok := s.opLocs[projectId][changeId][pathHash]; !ok {
		s.opLocs[projectId][changeId][pathHash] = &pb.OperationLocations{OpLocs: make([]*pb.OperationLocations_OperationLocation, 0)}
	}
	return s.opLocs[projectId][changeId][pathHash], nil
}
