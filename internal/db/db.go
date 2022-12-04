package db

import (
	"database/sql"
	"time"
)

func Setup(db *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (username TEXT);
	CREATE TABLE IF NOT EXISTS projects (name TEXT);
	CREATE TABLE IF NOT EXISTS changes (project_id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS change_data (change_id INTEGER, path TEXT, offset INTEGER, length INTEGER);
	`
	_, err := db.Exec(sqlStmt)
	return err
}

type Project struct {
	Name    string
	OwnerId uint64
	Id      uint64
}

func AddProject(db *sql.DB, projectName string) (uint64, error) {
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

func GetProject(db *sql.DB, projectName string) (string, uint64, error) {
	row := db.QueryRow("SELECT name FROM projects WHERE name = ?", projectName)
	if row.Err() != nil {
		return "", 0, row.Err()
	}

	var name string
	var ownerUserId uint64
	err := row.Scan(&name, &ownerUserId)
	return name, uint64(ownerUserId), err
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
		err = rows.Scan(&u.Id, &u.Name, &u.OwnerId)
		if err != nil {
			return nil, err
		}
		data = append(data, u)
	}
	return data, err
}

func AddChangeData(db *sql.DB, changeId uint64, path string, offset int64, length int) (uint64, error) {
	res, err := db.Exec("INSERT INTO change_data(change_id, path, offset, length) VALUES(?, ?, ?)", changeId, path, offset, length)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func AddChange(db *sql.DB, projectName string) (uint64, error) {
	res, err := db.Exec("INSERT INTO changes(project_id) SELECT project_id FROM projects WHERE project_name = ?", projectName)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func ListChanges(db *sql.DB, projectName string, timestamp time.Time) ([]uint64, []time.Time, []uint64, []uint64, error) {
	rows, err := db.Query("SELECT rowid, timestamp, offset, length FROM changes WHERE project_id = (SELECT rowid FROM projects WHERE project_name = ?) AND timestamp <= ?", projectName, timestamp)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if rows.Err() != nil {
		return nil, nil, nil, nil, rows.Err()
	}
	defer rows.Close()

	ids := make([]uint64, 0)
	times := make([]time.Time, 0)
	offsets := make([]uint64, 0)
	lengths := make([]uint64, 0)
	for rows.Next() {
		if rows.Err() != nil {
			return nil, nil, nil, nil, rows.Err()
		}
		var id uint64
		var time time.Time
		var offset uint64
		var length uint64
		err = rows.Scan(&id, &time, &offset, &length)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		ids = append(ids, id)
		times = append(times, time)
		offsets = append(offsets, offset)
		lengths = append(lengths, length)
	}
	return ids, times, offsets, lengths, err
}
