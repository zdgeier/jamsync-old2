package db

import (
	"database/sql"
)

func Setup(db *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (username TEXT);
	CREATE TABLE IF NOT EXISTS projects (name TEXT, owner_user_id INTEGER);
	CREATE TABLE IF NOT EXISTS branches (project_id INTEGER, name TEXT);
	CREATE TABLE IF NOT EXISTS history (branch_id INTEGER NOT NULL, user_id INTEGER);
	`
	_, err := db.Exec(sqlStmt)
	return err
}

type Project struct {
	Name    string
	OwnerId int64
	Id      int64
}

func AddProject(db *sql.DB, projectName string, userId int64) (int64, error) {
	res, err := db.Exec("INSERT INTO projects(name, owner_user_id) VALUES(?, ?)", projectName, userId)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return id, nil
}

func GetProject(db *sql.DB, projectId int64) (string, int64, error) {
	row := db.QueryRow("SELECT name, owner_user_id FROM projects WHERE rowid = ?", projectId)
	if row.Err() != nil {
		return "", -1, row.Err()
	}

	var name string
	var ownerUserId int64
	err := row.Scan(&name, &ownerUserId)
	return name, ownerUserId, err
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

func AddUser(db *sql.DB, username string) (int64, error) {
	res, err := db.Exec("INSERT INTO users(username) VALUES(?)", username)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return id, nil
}

type User struct {
	Id       int64
	Username string
}

func GetUser(db *sql.DB, userId int64) (string, error) {
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
