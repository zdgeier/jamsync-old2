package changestore

import (
	"database/sql"
	"errors"
	"time"

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/jamenv"
)

type ChangeStore interface {
	AddChange(projectId uint64) (uint64, error)
	GetCurrentChange(projectId uint64) (uint64, time.Time, error)
	CommitChange(projectId uint64, changeId uint64) error
	ListCommittedChanges(projectId uint64, pathHash uint64, timestamp time.Time) ([]uint64, error)
	AddOperationLocation(data *pb.OperationLocation) (uint64, error)
	ListOperationLocations(projectId uint64, pathHash uint64, changeId uint64) ([]*pb.OperationLocation, error)
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

func setup(db *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS committed_changes (change_id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS changes (id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS operation_locations (change_id INTEGER, path_hash INTEGER, offset INTEGER, length INTEGER);
	`
	_, err := db.Exec(sqlStmt)
	return err
}
func getCurrentChange(db *sql.DB) (uint64, time.Time, error) {
	row := db.QueryRow("SELECT c.id, c.timestamp FROM changes AS c ORDER BY c.id DESC LIMIT 1")
	if row.Err() != nil {
		return 0, time.Time{}, row.Err()
	}

	var id uint64
	var timestamp time.Time
	err := row.Scan(&id, &timestamp)
	if errors.Is(sql.ErrNoRows, err) {
		return 0, time.Time{}, nil
	}
	return id, timestamp, err
}

func addOperationLocation(db *sql.DB, changeId, pathHash, offset, length uint64) (uint64, error) {
	res, err := db.Exec("INSERT INTO operation_locations(change_id, path_hash, offset, length) VALUES(?, ?, ?, ?)", changeId, int64(pathHash), offset, length)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func addChange(db *sql.DB) (uint64, error) {
	changeId, _, err := getCurrentChange(db)
	if !errors.Is(sql.ErrNoRows, err) && err != nil {
		return 0, err
	}
	newId := changeId + 1
	_, err = db.Exec("INSERT INTO changes(id) VALUES(?)", newId)
	if err != nil {
		return 0, err
	}

	return uint64(newId), nil
}

func commitChange(db *sql.DB, changeId uint64) error {
	_, err := db.Exec("INSERT INTO committed_changes(change_id) VALUES(?)", changeId)
	if err != nil {
		return err
	}

	return err
}

func listCommittedChanges(db *sql.DB, pathHash uint64, timestamp time.Time) ([]uint64, error) {
	rows, err := db.Query("SELECT c.change_id FROM committed_changes AS c INNER JOIN operation_locations AS o WHERE c.change_id = o.change_id AND path_hash = ? AND timestamp < ? ORDER BY c.timestamp ASC", int64(pathHash), timestamp)
	if err != nil {
		return nil, err
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	defer rows.Close()

	changeIds := make([]uint64, 0)
	for rows.Next() {
		var changeId uint64
		rows.Scan(&changeId)
		changeIds = append(changeIds, changeId)
	}
	return changeIds, nil
}

func listOperationLocations(db *sql.DB, projectId uint64, pathHash uint64, changeId uint64) ([]*pb.OperationLocation, error) {
	rows, err := db.Query("SELECT offset, length FROM operation_locations WHERE path_hash = ? AND change_id = ?", int64(pathHash), changeId)
	if err != nil {
		return nil, err
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	defer rows.Close()

	operationLocations := make([]*pb.OperationLocation, 0)
	for rows.Next() {
		var offset, length uint64
		rows.Scan(&offset, &length)
		operationLocations = append(operationLocations, &pb.OperationLocation{
			ProjectId: projectId,
			ChangeId:  changeId,
			PathHash:  pathHash,
			Offset:    offset,
			Length:    length,
		})
	}
	return operationLocations, nil
}
