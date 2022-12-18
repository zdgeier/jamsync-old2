package server

import (
	"database/sql"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/changestore"
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.UnimplementedJamsyncAPIServer
	store changestore.ChangeStore
}

func NewServer(db *sql.DB) JamsyncServer {
	server := JamsyncServer{
		db: db,
	}

	return server
}
