package server

import (
	"context"
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

func Addr() string {
	switch jamenv.Env() {
	case jamenv.Memory:
		panic("trying to get address of inmemory server")
	default:
		return "0.0.0.0:14357"
	}
}

func New() (client pb.JamsyncAPIClient, closer func(), err error) {
	var lis *bufconn.Listener
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
		tcplis, err := net.Listen("tcp", Addr())
		if err != nil {
			return nil, nil, err
		}
		go func() {
			if err := server.Serve(tcplis); err != nil {
				log.Printf("error serving server: %v", err)
			}
		}()

		conn, err := grpc.Dial(Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Panicf("could not connect to jamsync server: %s", err)
		}
		defer conn.Close()
		client = pb.NewJamsyncAPIClient(conn)

		closer = func() {
			err := tcplis.Close()
			if err != nil {
				log.Printf("error closing listener: %v", err)
			}
			server.Stop()
		}
	case jamenv.Memory:
		buffer := 101024 * 1024
		lis = bufconn.Listen(buffer)
		go func() {
			if err := server.Serve(lis); err != nil {
				log.Printf("error serving server: %v", err)
			}
		}()

		conn, err := grpc.DialContext(context.Background(), "",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return lis.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("error connecting to server: %v", err)
		}
		client = pb.NewJamsyncAPIClient(conn)

		closer = func() {
			// err := lis.Close()
			// if err != nil {
			// 	log.Printf("error closing listener: %v", err)
			// }
			server.Stop()
		}
	}

	return client, closer, nil
}
