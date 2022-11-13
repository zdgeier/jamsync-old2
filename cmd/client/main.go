package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var serverAddr = flag.String("addr", "localhost:14357", "The server address in the format of host:port")

func main() {
	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := jamsyncpb.NewJamsyncAPIClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.AddUser(ctx, &jamsyncpb.AddUserRequest{Username: "zdgeier"})
	if err != nil {
		panic(err)
	}
	getResp, err := client.GetUser(ctx, &jamsyncpb.GetUserRequest{UserId: resp.GetUserId()})
	if err != nil {
		panic(err)
	}
	fmt.Println("Got", getResp.GetUsername())

	stream, err := client.UpdateStream(context.Background())
	if err != nil {
		panic(err)
	}

	waitc := make(chan struct{})
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				// read done.
				close(waitc)
				return
			}
			if err != nil {
				log.Fatalf("Failed to receive a note : %v", err)
			}
			log.Printf("Got message %s", in.Data)
		}
	}()

	watcher, _ := fsnotify.NewWatcher()
	defer watcher.Close()

	// starting at the root of the project, walk each file/directory searching for
	// directories
	if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, _ error) error {
		log.Println("Watching", path)
		return watcher.Add(path)
	}); err != nil {
		log.Panic("Could not walk directory tree to watch files", err)
	}

	// When changes come from local
	for {
		select {
		case event := <-watcher.Events:
			if event.Op == fsnotify.Chmod {
				continue
			}

			path := event.Name

			if stat, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				log.Println(path + " deleted")
				err := watcher.Remove(path)
				if err != nil {
					log.Fatal(err)
				}
			} else if stat.IsDir() {
				if err := filepath.WalkDir(path, func(path string, d fs.DirEntry, _ error) error {
					if d.IsDir() {
						log.Println(path + " directory changed")
					} else {
						log.Println(path + " changed")
					}

					return watcher.Add(path)
				}); err != nil {
					log.Panic("Could not walk directory tree to watch files")
				}
			} else {
				log.Println(path + " changed ")
				err := watcher.Add(path)
				if err != nil {
					log.Fatal(err)
				}
			}

			stream.Send(&jamsyncpb.UpdateStreamRequest{Data: []byte("send it")})
			if err != nil {
				log.Fatal(err)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}

	stream.CloseSend()
	<-waitc
}
