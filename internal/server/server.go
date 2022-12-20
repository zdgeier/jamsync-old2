package server

import (
	"database/sql"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
)

type JamsyncServer struct {
	db *sql.DB
	jamsyncpb.JamsyncAPIServer
	storeClient jamsyncpb.JamsyncStoreClient
}

func NewServer(db *sql.DB, storeClient jamsyncpb.JamsyncStoreClient) JamsyncServer {
	server := JamsyncServer{
		db:          db,
		storeClient: storeClient,
	}

	return server
}
