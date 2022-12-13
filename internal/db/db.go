package db

import (
	"database/sql"
	"errors"
	"time"
)

func Setup(db *sql.DB) error {
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (username TEXT);
	CREATE TABLE IF NOT EXISTS projects (name TEXT);
	CREATE TABLE IF NOT EXISTS changes (id INTEGER, project_id INTEGER, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS change_data (change_id INTEGER, path TEXT, offset INTEGER, length INTEGER);
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
	if row.Err() != nil {
		return 0, time.Time{}, row.Err()
	}

	var id uint64
	var timestamp time.Time
	err := row.Scan(&id, &timestamp)
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

func AddChangeData(db *sql.DB, changeId uint64, path string, offset int64, length int) (uint64, error) {
	res, err := db.Exec("INSERT INTO change_data(change_id, path, offset, length) VALUES(?, ?, ?, ?)", changeId, path, offset, length)
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
	res, err := db.Exec("INSERT INTO changes(id, project_id) SELECT ?, rowid FROM projects WHERE name = ?", changeId+1, projectName)
	if err != nil {
		return 0, err
	}

	var id int64
	if id, err = res.LastInsertId(); err != nil {
		return 0, err
	}

	return uint64(id), nil
}

// func ListChanges(db *sql.DB, projectName string, timestamp time.Time) ([]uint64, []time.Time, error) {
// 	rows, err := db.Query("SELECT rowid, timestamp FROM changes AS c INNER JOIN projects AS p WHERE p.project_name = ? AND c.timestamp <= ?", projectName, timestamp)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	if rows.Err() != nil {
// 		return nil, nil, rows.Err()
// 	}
// 	defer rows.Close()
//
// 	ids := make([]uint64, 0)
// 	times := make([]time.Time, 0)
// 	for rows.Next() {
// 		if rows.Err() != nil {
// 			return nil, nil, rows.Err()
// 		}
// 		var id uint64
// 		var time time.Time
// 		err = rows.Scan(&id, &time)
// 		if err != nil {
// 			return nil, nil, err
// 		}
// 		ids = append(ids, id)
// 		times = append(times, time)
// 	}
// 	return ids, times, err
// }

func ListChangeDataForPath(db *sql.DB, projectName string, path string) ([]uint64, []uint64, error) {
	rows, err := db.Query("SELECT offset, length FROM change_data INNER JOIN projects AS p INNER JOIN changes AS c WHERE p.name = ? AND p.rowid = c.project_id AND change_id = c.rowid AND path = ?", projectName, path)
	if err != nil {
		return nil, nil, err
	}
	if rows.Err() != nil {
		return nil, nil, rows.Err()
	}
	defer rows.Close()

	offsets := make([]uint64, 0)
	lengths := make([]uint64, 0)
	for rows.Next() {
		if rows.Err() != nil {
			return nil, nil, rows.Err()
		}
		var offset uint64
		var length uint64
		err = rows.Scan(&offset, &length)
		if err != nil {
			return nil, nil, err
		}
		offsets = append(offsets, offset)
		lengths = append(lengths, length)
	}
	return offsets, lengths, nil
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
