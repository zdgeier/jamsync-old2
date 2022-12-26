package server

import (
	"context"
	"database/sql"
	"log"
	"net"

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
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

func NewLocalServer(ctx context.Context, db *sql.DB, store store.Store) (pb.JamsyncAPIClient, func()) {
	buffer := 101024 * 1024
	lis := bufconn.Listen(buffer)

	baseServer := grpc.NewServer()
	pb.RegisterJamsyncAPIServer(baseServer, NewServer(db, store))
	go func() {
		if err := baseServer.Serve(lis); err != nil {
			log.Printf("error serving server: %v", err)
		}
	}()

	conn, err := grpc.DialContext(ctx, "",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("error connecting to server: %v", err)
	}

	closer := func() {
		err := lis.Close()
		if err != nil {
			log.Printf("error closing listener: %v", err)
		}
		baseServer.Stop()
	}

	client := pb.NewJamsyncAPIClient(conn)

	return client, closer
}
