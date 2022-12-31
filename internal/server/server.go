package server

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/changestore"
	"github.com/zdgeier/jamsync/internal/db"
	"github.com/zdgeier/jamsync/internal/jamenv"
	"github.com/zdgeier/jamsync/internal/opstore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/test/bufconn"
)

type JamsyncServer struct {
	db          db.JamsyncDb
	opstore     opstore.OpStore
	changestore changestore.ChangeStore
	pb.UnimplementedJamsyncAPIServer
}

var memBuffer *bufconn.Listener

func New() (closer func(), err error) {
	jamsyncServer := JamsyncServer{
		db:          db.New(),
		opstore:     opstore.New(),
		changestore: changestore.New(),
	}

	server := grpc.NewServer()
	reflection.Register(server)
	pb.RegisterJamsyncAPIServer(server, jamsyncServer)

	switch jamenv.Env() {
	case jamenv.Prod, jamenv.Dev, jamenv.Local:
		tcplis, err := net.Listen("tcp", "0.0.0.0:14357")
		if err != nil {
			return nil, err
		}
		go func() {
			if err := server.Serve(tcplis); err != nil {
				log.Printf("error serving server: %v", err)
			}
		}()
	case jamenv.Memory:
		buffer := 101024 * 1024
		memBuffer = bufconn.Listen(buffer)
		go func() {
			if err := server.Serve(memBuffer); err != nil {
				log.Printf("error serving server: %v", err)
			}
		}()
	}

	return func() { server.Stop() }, nil
}

func Connect() (client pb.JamsyncAPIClient, closer func(), err error) {
	switch jamenv.Env() {
	case jamenv.Memory:
		conn, err := grpc.DialContext(context.Background(), "",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return memBuffer.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("error connecting to server: %v", err)
		}
		client = pb.NewJamsyncAPIClient(conn)
		closer = func() {
			if err := conn.Close(); err != nil {
				log.Panic("could not close server connection")
			}
		}
	default:
		conn, err := grpc.Dial(jamenv.PublicAPIAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			raddr, err := net.ResolveTCPAddr("tcp", jamenv.PublicAPIAddress())
			if err != nil {
				return nil, err
			}

			conn, err := net.DialTCP("tcp", nil, raddr)
			if err != nil {
				return nil, err
			}

			file, err := conn.File()
			if err != nil {
				return nil, err
			}
			fmt.Println("Connection", file.Name())

			return conn, err
		}))
		if err != nil {
			log.Panicf("could not connect to jamsync server: %s", err)
		}
		client = pb.NewJamsyncAPIClient(conn)
		closer = func() {
			if err := conn.Close(); err != nil {
				log.Panic("could not close server connection")
			}
		}
	}

	return client, closer, err
}
