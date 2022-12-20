package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"
	"net"

	_ "github.com/mattn/go-sqlite3"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

var port = flag.Int("port", 14357, "port to start the grpc server")
var storeAddr = flag.String("storeAddr", "localhost:14358", "address of the store grpc server")

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

	// TODO: secure
	conn, err := grpc.Dial(*storeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Panicf("could not connect to jamsync server: %s", err)
	}
	defer conn.Close()

	storeClient := jamsyncpb.NewJamsyncStoreClient(conn)
	jamsyncServer := server.NewServer(localDB, storeClient)

	jamsyncpb.RegisterJamsyncAPIServer(grpcServer, jamsyncServer)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Panic("stopping", err)
	}
}
