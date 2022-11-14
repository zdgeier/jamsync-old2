package db

import (
	"database/sql"
	"fmt"
	"log"
)

func Setup(db *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (username TEXT);
	CREATE TABLE IF NOT EXISTS projects (name TEXT, owner_user_id INTEGER);
	CREATE TABLE IF NOT EXISTS branches (project_id INTEGER, name TEXT);
	CREATE TABLE IF NOT EXISTS changes (branch_id INTEGER NOT NULL, user_id INTEGER, project_id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS manifest_changes (branch_id INTEGER NOT NULL, user_id INTEGER, project_id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
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
	res, err := db.Exec("INSERT INTO projects(name, owner_user_id) VALUES(?, ?)", projectName, userId)
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
	row := db.QueryRow("SELECT name, owner_user_id FROM projects WHERE rowid = ?", projectId)
	if row.Err() != nil {
		return "", 0, row.Err()
	}

	var name string
	var ownerUserId uint64
	err := row.Scan(&name, &ownerUserId)
	return name, uint64(ownerUserId), err
}

func ListProjects(db *sql.DB) ([]Project, error) {
	rows, err := db.Query("SELECT rowid, name, owner_user_id FROM projects")
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

func AddChange(db *sql.DB, branchId uint64, userId uint64, projectId uint64) (uint64, error) {
	res, err := db.Exec("INSERT INTO changes(branch_id, user_id, project_id) VALUES(?, ?, ?)", branchId, userId, projectId)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func ListChanges(db *sql.DB, branchId uint64, projectId uint64) ([]uint64, error) {
	fmt.Println("01")
	rows, err := db.Query("SELECT rowid FROM changes WHERE branch_id = ? AND project_id = ?", branchId, projectId)
	if err != nil {
		log.Fatal("asdf")
		return nil, err
	}
	defer rows.Close()

	fmt.Println("11")
	ids := make([]uint64, 0)
	for rows.Next() {
		var id uint64
		err = rows.Scan(&id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	fmt.Println("12")
	return ids, err
}

func AddManifestChange(db *sql.DB, branchId uint64, userId uint64, projectId uint64) (uint64, error) {
	res, err := db.Exec("INSERT INTO manifest_changes(branch_id, user_id, project_id) VALUES(?, ?, ?)", branchId, userId, projectId)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func ListManifestChanges(db *sql.DB, branchId uint64, projectId uint64) ([]uint64, error) {
	rows, err := db.Query("SELECT rowid FROM manifest_changes WHERE branch_id = ? AND project_id = ?", branchId, projectId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]uint64, 0)
	for rows.Next() {
		var id uint64
		err = rows.Scan(&id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, err
}
