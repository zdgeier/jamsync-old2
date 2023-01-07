package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/zdgeier/jamsync/internal/jamenv"
)

type JamsyncDb struct {
	db *sql.DB
}

func New() (jamsyncDB JamsyncDb) {
	var db *sql.DB
	switch jamenv.Env() {
	case jamenv.Prod, jamenv.Dev, jamenv.Local:
		conn, err := sql.Open("sqlite3", "./jamsync.db")
		if err != nil {
			panic(err)
		}
		db = conn
	case jamenv.Memory:
		conn, err := sql.Open("sqlite3", "file:foobar?mode=memory&cache=shared")
		if err != nil {
			panic(err)
		}
		db = conn
	}

	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (username TEXT, user_id TEXT, UNIQUE(username, user_id));
	CREATE TABLE IF NOT EXISTS projects (name TEXT, owner TEXT);
	`
	_, err := db.Exec(sqlStmt)
	if err != nil {
		panic(err)
	}
	return JamsyncDb{db}
}

type Project struct {
	Name string
	Id   uint64
}

func (j JamsyncDb) AddProject(projectName string, owner string) (uint64, error) {
	_, err := j.GetProjectId(projectName, owner)
	fmt.Println("asdf", err)
	if !errors.Is(sql.ErrNoRows, err) {
		return 0, fmt.Errorf("project already exists")
	}

	res, err := j.db.Exec("INSERT INTO projects(name, owner) VALUES(?, ?)", projectName, owner)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func (j JamsyncDb) GetProjectOwner(projectId uint64) (string, error) {
	row := j.db.QueryRow("SELECT owner FROM projects WHERE rowid = ?", projectId)
	if row.Err() != nil {
		return "", row.Err()
	}

	var owner string
	err := row.Scan(&owner)
	return owner, err
}

func (j JamsyncDb) GetProjectId(projectName string, owner string) (uint64, error) {
	fmt.Println("PRoject", projectName, owner)
	row := j.db.QueryRow("SELECT rowid FROM projects WHERE name = ? AND owner = ?", projectName, owner)
	if row.Err() != nil {
		return 0, row.Err()
	}

	var id uint64
	err := row.Scan(&id)
	return id, err
}

func (j JamsyncDb) ListUserProjects(owner string) ([]Project, error) {
	rows, err := j.db.Query("SELECT rowid, name FROM projects WHERE owner = ?", owner)
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

func (j JamsyncDb) ListProjects() ([]Project, error) {
	rows, err := j.db.Query("SELECT rowid, name FROM projects WHERE owner = auth0|63b5ad0c396bd87d2aff62df")
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

func (j JamsyncDb) CreateUser(username, userId string) error {
	_, err := j.db.Exec("INSERT OR IGNORE INTO users(username, user_id) VALUES (?, ?)", username, userId)
	return err
}
