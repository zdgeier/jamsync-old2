package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	_ "github.com/mattn/go-sqlite3"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var port = flag.Int("port", 14358, "port to start the grpc server")

func main() {
	flag.Parse()
	address := fmt.Sprintf("localhost:%d", *port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Panicf("failed to listen: %v", err)
	}
	fmt.Println("Starting to listen on", address)
	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer)

	jamsyncStore := store.NewStore()

	jamsyncpb.RegisterJamsyncStoreServer(grpcServer, jamsyncStore)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Panic("stopping", err)
	}
}
