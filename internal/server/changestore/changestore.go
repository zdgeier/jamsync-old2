package changestore

import (
	"time"

	"github.com/zdgeier/jamsync/internal/jamenv"
)

type ChangeStore interface {
	AddChange(projectId uint64, ownerId uint64) (uint64, error)
	GetCurrentChange(projectId uint64, ownerId uint64) (uint64, time.Time, error)
	CommitChange(projectId uint64, ownerId uint64, changeId uint64) error
	ListCommittedChanges(projectId uint64, ownerId uint64, timestamp time.Time) ([]uint64, error)
}

func New() ChangeStore {
	var changeStore ChangeStore
	switch jamenv.Env() {
	case jamenv.Prod, jamenv.Dev, jamenv.Local:
		changeStore = NewLocalChangeStore()
	case jamenv.Memory:
		changeStore = NewMemoryChangeStore()
	}
	return changeStore
}
