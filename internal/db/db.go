package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"google.golang.org/protobuf/proto"
)

func Setup(db *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (username TEXT);
	CREATE TABLE IF NOT EXISTS projects (name TEXT);
	CREATE TABLE IF NOT EXISTS changes (id INTEGER, project_id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS change_data (change_id INTEGER, pathHash INTEGER, locationList BLOB);
	`
	_, err := db.Exec(sqlStmt)
	return err
}

type Project struct {
	Name string
	Id   uint64
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

func GetProjectId(db *sql.DB, projectName string) (uint64, error) {
	row := db.QueryRow("SELECT rowid FROM projects WHERE name = ?", projectName)
	if row.Err() != nil {
		return 0, row.Err()
	}

	var id uint64
	err := row.Scan(&id)
	return id, err
}

func GetCurrentChange(db *sql.DB, projectName string) (uint64, time.Time, error) {
	row := db.QueryRow("SELECT c.id, c.timestamp FROM changes AS c INNER JOIN projects AS p WHERE p.name = ? ORDER BY c.timestamp DESC LIMIT 1", projectName)
	fmt.Println("test3")
	if row.Err() != nil {
		fmt.Println("test2")
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

func AddChangeLocationList(db *sql.DB, data *jamsyncpb.ChangeLocationList) (uint64, error) {
	locationListData, err := proto.Marshal(data)
	if err != nil {
		return 0, err
	}

	res, err := db.Exec("INSERT INTO change_data(change_id, pathHash, locationList) VALUES(?, ?, ?)", data.ChangeId, data.PathHash, locationListData)
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
	changeId, _, err := GetCurrentChange(db, projectName)
	if !errors.Is(sql.ErrNoRows, err) && err != nil {
		return 0, err
	}
	projectId, err := GetProjectId(db, projectName)
	if err != nil {
		return 0, err
	}
	newId := changeId + 1
	_, err = db.Exec("INSERT INTO changes(id, project_id) VALUES(?, ?)", newId, projectId)
	if err != nil {
		return 0, err
	}

	return uint64(newId), nil
}

func ChangeLocationLists(db *sql.DB, projectName string, pathHash uint64, timestamp time.Time) ([]*jamsyncpb.ChangeLocationList, error) {
	fmt.Println(projectName, pathHash, timestamp)
	rows, err := db.Query("SELECT locationList FROM change_data INNER JOIN projects AS p INNER JOIN changes AS c WHERE p.name = ? AND p.rowid = c.project_id AND change_id = c.id AND pathHash = ? AND c.timestamp < ?", projectName, pathHash, timestamp)
	if err != nil {
		return nil, err
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	defer rows.Close()

	changeLocationLists := make([]*jamsyncpb.ChangeLocationList, 0)
	for rows.Next() {
		if rows.Err() != nil {
			return nil, rows.Err()
		}

		var changeLocationListData []byte
		err = rows.Scan(&changeLocationListData)
		if err != nil {
			return nil, err
		}

		changeLocationList := jamsyncpb.ChangeLocationList{}
		err := proto.Unmarshal(changeLocationListData, &changeLocationList)
		if err != nil {
			return nil, err
		}
		fmt.Println("GOT", changeLocationList)
		changeLocationLists = append(changeLocationLists, &changeLocationList)
	}
	return changeLocationLists, nil
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
