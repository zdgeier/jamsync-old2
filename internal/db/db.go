package db

import (
	"database/sql"
	"time"
)

func Setup(db *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (username TEXT);
	CREATE TABLE IF NOT EXISTS projects (name TEXT);
	CREATE TABLE IF NOT EXISTS branches (project_id INTEGER, name TEXT);
	CREATE TABLE IF NOT EXISTS changes (branch_id INTEGER NOT NULL, project_id INTEGER, offset INTEGER, length INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	`
	_, err := db.Exec(sqlStmt)
	return err
}

type Project struct {
	Name    string
	OwnerId uint64
	Id      uint64
}

func AddProject(db *sql.DB, projectName string, userId uint64) (uint64, error) {
	res, err := db.Exec("INSERT INTO projects(name) VALUES(?, ?)", projectName, userId)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func GetProject(db *sql.DB, projectId uint64) (string, uint64, error) {
	row := db.QueryRow("SELECT name FROM projects WHERE rowid = ?", projectId)
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

func AddUser(db *sql.DB, username string) (uint64, error) {
	res, err := db.Exec("INSERT INTO users(username) VALUES(?)", username)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

type User struct {
	Id       uint64
	Username string
}

func GetUser(db *sql.DB, userId uint64) (string, error) {
	row := db.QueryRow("SELECT username FROM users WHERE rowid = ?", userId)
	if row.Err() != nil {
		return "", row.Err()
	}

	var username string
	err := row.Scan(&username)
	return username, err
}

func ListUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query("SELECT rowid, username FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	data := make([]User, 0)
	for rows.Next() {
		u := User{}
		err = rows.Scan(&u.Id, &u.Username)
		if err != nil {
			return nil, err
		}
		data = append(data, u)
	}
	return data, err
}

func AddChange(db *sql.DB, branchId uint64, projectId uint64, offset int64, length int64) (uint64, error) {
	res, err := db.Exec("INSERT INTO changes(branch_id, project_id, offset, length) VALUES(?, ?, ?, ?)", branchId, projectId, offset, length)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func ListChanges(db *sql.DB, branchId uint64, projectId uint64, timestamp time.Time) ([]uint64, []time.Time, []uint64, []uint64, error) {
	rows, err := db.Query("SELECT rowid, timestamp, offset, length FROM changes WHERE branch_id = ? AND project_id = ? AND timestamp <= ?", branchId, projectId, timestamp)
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
