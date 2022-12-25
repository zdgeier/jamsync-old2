package server

import (
	"database/sql"

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/store"
)

type JamsyncServer struct {
	db    *sql.DB
	store store.Store
	pb.UnimplementedJamsyncAPIServer
}

func NewServer(db *sql.DB, store store.Store) JamsyncServer {
	server := JamsyncServer{
		db:    db,
		store: store,
	}

	return server
}
