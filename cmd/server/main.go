package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"
	"net"

	_ "github.com/mattn/go-sqlite3"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/server"
	"github.com/zdgeier/jamsync/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var port = flag.Int("port", 14357, "port to start the grpc server")

func main() {
	// err := syscall.Chroot(".")
	// if err != nil {
	// 	log.Println("Could not chroot current directory. Run with `sudo` to allow chroot.")
	// }

	// os.Remove("./jamsync.db")
	localDB, err := sql.Open("sqlite3", "./jamsync.db")
	if err != nil {
		log.Panic(err)
	}
	db.Setup(localDB)

	flag.Parse()
	address := fmt.Sprintf("localhost:%d", *port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Panicf("failed to listen: %v", err)
	}
	fmt.Println("Starting to listen on", address)
	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(math.MaxInt32)}
	grpcServer := grpc.NewServer(opts...)
	reflection.Register(grpcServer)

	jamsyncServer := server.NewServer(localDB, store.NewLocalStore("jb"))

	pb.RegisterJamsyncAPIServer(grpcServer, jamsyncServer)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Panic("stopping", err)
	}
}
