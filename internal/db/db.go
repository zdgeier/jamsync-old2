package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zdgeier/jamsync/gen/pb"
)

func Setup(db *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (username TEXT);
	CREATE TABLE IF NOT EXISTS projects (name TEXT);
	CREATE TABLE IF NOT EXISTS committed_changes (project_id INTEGER, change_id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS changes (id INTEGER, project_id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS operation_locations (project_id INTEGER, change_id INTEGER, path_hash INTEGER, offset INTEGER, length INTEGER);
	`
	_, err := db.Exec(sqlStmt)
	return err
}

type Project struct {
	Name string
	Id   uint64
}

func AddProject(db *sql.DB, projectName string) (uint64, error) {
	_, err := GetProjectId(db, projectName)
	if !errors.Is(sql.ErrNoRows, err) {
		return 0, fmt.Errorf("project already exists")
	}

	res, err := db.Exec("INSERT INTO projects(name) VALUES(?)", projectName)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func GetProjectId(db *sql.DB, projectName string) (uint64, error) {
	row := db.QueryRow("SELECT rowid FROM projects WHERE name = ?", projectName)
	if row.Err() != nil {
		return 0, row.Err()
	}

	var id uint64
	err := row.Scan(&id)
	return id, err
}

func GetCurrentChange(db *sql.DB, projectId uint64) (uint64, time.Time, error) {
	row := db.QueryRow("SELECT c.id, c.timestamp FROM changes AS c INNER JOIN projects AS p WHERE p.rowid = ? ORDER BY c.id DESC LIMIT 1", projectId)
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

func ListProjects(db *sql.DB) ([]Project, error) {
	rows, err := db.Query("SELECT rowid, name FROM projects")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	data := make([]Project, 0)
	for rows.Next() {
		u := Project{}
		err = rows.Scan(&u.Id, &u.Name)
		if err != nil {
			return nil, err
		}
		data = append(data, u)
	}
	return data, err
}

func AddOperationLocation(db *sql.DB, data *pb.OperationLocation) (uint64, error) {
	res, err := db.Exec("INSERT INTO operation_locations(project_id, change_id, path_hash, offset, length) VALUES(?, ?, ?, ?, ?)", data.ProjectId, data.ChangeId, int64(data.PathHash), data.Offset, data.Length)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func AddChange(db *sql.DB, projectId uint64) (uint64, error) {
	changeId, _, err := GetCurrentChange(db, projectId)
	if !errors.Is(sql.ErrNoRows, err) && err != nil {
		return 0, err
	}
	newId := changeId + 1
	_, err = db.Exec("INSERT INTO changes(id, project_id) VALUES(?, ?)", newId, projectId)
	if err != nil {
		return 0, err
	}

	return uint64(newId), nil
}

func CommitChange(db *sql.DB, projectId uint64, changeId uint64) error {
	fmt.Println("comming", projectId, changeId)
	_, err := db.Exec("INSERT INTO committed_changes(change_id, project_id) VALUES(?, ?)", changeId, projectId)
	if err != nil {
		return err
	}

	return err
}

func ListCommittedChanges(db *sql.DB, projectId uint64, pathHash uint64, timestamp time.Time) ([]uint64, error) {
	fmt.Println("LIST", projectId, pathHash, timestamp)
	rows, err := db.Query("SELECT c.change_id FROM committed_changes AS c INNER JOIN operation_locations AS o WHERE c.project_id = ? AND c.change_id = o.change_id AND path_hash = ? AND timestamp < ? ORDER BY c.timestamp ASC", projectId, int64(pathHash), timestamp)
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

func ListOperationLocations(db *sql.DB, projectId uint64, pathHash uint64, changeId uint64) ([]*pb.OperationLocation, error) {
	rows, err := db.Query("SELECT offset, length FROM operation_locations WHERE project_id = ? AND path_hash = ? AND change_id = ?", projectId, int64(pathHash), changeId)
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

func CreateUser(db *sql.DB, username string) (uint64, error) {
	res, err := db.Exec("INSERT INTO users(username) VALUES (?)", username)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}
