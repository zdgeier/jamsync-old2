package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"runtime/trace"
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

	//os.Remove("./jamsync.db")
	localDB, err := sql.Open("sqlite3", "./jamsync.db")
	if err != nil {
		log.Fatal(err)
	}
	db.Setup(localDB)

	errChan := make(chan error)
	stopChan := make(chan os.Signal)

	// bind OS events to the signal channel
	signal.Notify(stopChan, syscall.SIGTERM, syscall.SIGINT)
	fmt.Println("profiling")
	p, err := os.Create("test.prof")
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(p)
	//trace.Start(p)

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
	go func() {
		err = grpcServer.Serve(lis)
		if err != nil {
			log.Fatal(err)
			errChan <- err
		}
	}()

	// terminate your environment gracefully before leaving main function
	defer func() {
		trace.Stop()
		//grpcServer.GracefulStop()
		pprof.StopCPUProfile()
	}()

	// block until either OS signal, or server fatal error
	select {
	case err := <-errChan:
		log.Printf("Fatal error: %v\n", err)
	case <-stopChan:
	}
}
