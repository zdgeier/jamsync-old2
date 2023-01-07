package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"fmt"
	"log"
	"net"

	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/jamenv"
	"github.com/zdgeier/jamsync/internal/server/changestore"
	"github.com/zdgeier/jamsync/internal/server/db"
	"github.com/zdgeier/jamsync/internal/server/oplocstore"
	"github.com/zdgeier/jamsync/internal/server/opstore"
	"github.com/zdgeier/jamsync/internal/server/serverauth"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/test/bufconn"
)

//go:embed publickey.cer
var f embed.FS

type JamsyncServer struct {
	db          db.JamsyncDb
	opstore     opstore.LocalStore
	oplocstore  oplocstore.LocalOpLocStore
	changestore changestore.LocalChangeStore
	pb.UnimplementedJamsyncAPIServer
}

var (
	memBuffer *bufconn.Listener
)

func New() (closer func(), err error) {
	jamsyncServer := JamsyncServer{
		db:          db.New(),
		opstore:     opstore.NewLocalStore("jb"),
		oplocstore:  oplocstore.NewLocalOpLocStore("jb"),
		changestore: changestore.NewLocalChangeStore(),
	}

	cert, err := tls.LoadX509KeyPair("/etc/jamsync/x509/publickey.cer", "/etc/jamsync/x509/private.key")
	if err != nil {
		log.Fatalf("failed to load key pair: %s", err)
	}
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(serverauth.EnsureValidToken),
		grpc.Creds(credentials.NewServerTLSFromCert(&cert)),
	}
	server := grpc.NewServer(opts...)
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

func Connect(accessToken *oauth2.Token) (client pb.JamsyncAPIClient, closer func(), err error) {
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
		perRPC := oauth.TokenSource{TokenSource: oauth2.StaticTokenSource(accessToken)}

		data, _ := f.ReadFile("publickey.crt")
		cp := x509.NewCertPool()
		cp.AppendCertsFromPEM(data)
		credentials.NewClientTLSFromCert(cp, "jamsync.dev")
		creds, err := credentials.NewClientTLSFromFile("/etc/jamsync/x509/publickey.cer", "jamsync.dev")
		if err != nil {
			log.Fatalf("failed to load credentials: %v", err)
		}
		opts := []grpc.DialOption{
			grpc.WithPerRPCCredentials(perRPC),
			grpc.WithTransportCredentials(creds),
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
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
			}),
		}
		conn, err := grpc.Dial(jamenv.PublicAPIAddress(), opts...)
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
