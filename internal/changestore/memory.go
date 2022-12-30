package changestore

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/zdgeier/jamsync/gen/pb"
)

type MemoryChangeStore struct {
	dbs map[uint64]*sql.DB
}

func NewMemoryChangeStore() MemoryChangeStore {
	return MemoryChangeStore{
		dbs: make(map[uint64]*sql.DB, 0),
	}
}

func (s MemoryChangeStore) getMemoryProjectDB(projectId uint64) (*sql.DB, error) {
	if db, ok := s.dbs[projectId]; ok {
		return db, nil
	}

	localDB, err := sql.Open("sqlite3", fmt.Sprintf("file:foobar%d?mode=memory&cache=shared", projectId))
	if err != nil {
		return nil, err
	}

	err = setup(localDB)
	if err != nil {
		return nil, err
	}

	s.dbs[projectId] = localDB
	return localDB, nil
}

func (s MemoryChangeStore) AddChange(projectId uint64) (uint64, error) {
	db, err := s.getMemoryProjectDB(projectId)
	if err != nil {
		return 0, err
	}
	return addChange(db)
}
func (s MemoryChangeStore) GetCurrentChange(projectId uint64) (uint64, time.Time, error) {
	db, err := s.getMemoryProjectDB(projectId)
	if err != nil {
		return 0, time.Time{}, err
	}
	return getCurrentChange(db)
}
func (s MemoryChangeStore) CommitChange(projectId uint64, changeId uint64) error {
	db, err := s.getMemoryProjectDB(projectId)
	if err != nil {
		return err
	}
	return commitChange(db, changeId)
}
func (s MemoryChangeStore) ListCommittedChanges(projectId uint64, pathHash uint64, timestamp time.Time) ([]uint64, error) {
	db, err := s.getMemoryProjectDB(projectId)
	if err != nil {
		return nil, err
	}
	return listCommittedChanges(db, pathHash, timestamp)
}
func (s MemoryChangeStore) AddOperationLocation(data *pb.OperationLocation) (uint64, error) {
	db, err := s.getMemoryProjectDB(data.ProjectId)
	if err != nil {
		return 0, err
	}
	return addOperationLocation(db, data.ChangeId, data.PathHash, data.Offset, data.Length)
}
func (s MemoryChangeStore) ListOperationLocations(projectId uint64, pathHash uint64, changeId uint64) ([]*pb.OperationLocation, error) {
	db, err := s.getMemoryProjectDB(projectId)
	if err != nil {
		return nil, err
	}
	return listOperationLocations(db, projectId, pathHash, changeId)
}
