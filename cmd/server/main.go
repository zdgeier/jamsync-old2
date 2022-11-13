package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var port = flag.Int("port", 14357, "port to start the grpc server")

func main() {
	err := syscall.Chroot(".")
	if err != nil {
		log.Println("Could not chroot current directory. Run with `sudo` to allow chroot.")
	}

	os.Remove("./jamsync.db")
	localDB, err := sql.Open("sqlite3", "./jamsync.db")
	if err != nil {
		log.Fatal(err)
	}
	db.Setup(localDB)

	flag.Parse()
	address := fmt.Sprintf("localhost:%d", *port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	fmt.Println("Starting to listen on", address)
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	reflection.Register(grpcServer)

	jamsyncServer := server.NewServer(localDB)
	jamsyncServer.GenTestData()

	jamsyncpb.RegisterJamsyncAPIServer(grpcServer, jamsyncServer)
	grpcServer.Serve(lis)
}
