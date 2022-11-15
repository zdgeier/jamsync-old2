package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/rsync"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var serverAddr = flag.String("addr", "localhost:14357", "The server address in the format of host:port")

func main() {
	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := jamsyncpb.NewJamsyncAPIClient(conn)

	for {
		f, err := os.OpenFile("test.txt",
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
		}
		if _, err := f.WriteString("text to appentext to appendtext to appendtext to appendtext to appendtext to appendtext to appendtext to appendtext to appendtext to appendtext to appendtext to appendtext to appendtext to appendtext to appendd\n"); err != nil {
			log.Println(err)
		}

		upload(client, "test.txt")
		f.Close()
	}

	// waitc := make(chan struct{})
	// go func() {
	// 	for {
	// 		in, err := stream.Recv()
	// 		if err == io.EOF {
	// 			// read done.
	// 			close(waitc)
	// 			return
	// 		}
	// 		if err != nil {
	// 			log.Fatalf("Failed to receive a note : %v", err)
	// 		}
	// 		log.Printf("Got message %s", in.Data)
	// 	}
	// }()
	// <-waitc
	watcher, _ := fsnotify.NewWatcher()
	defer watcher.Close()

	// starting at the root of the project, walk each file/directory searching for
	// directories
	if err := filepath.WalkDir("cmd/client/test", func(path string, d fs.DirEntry, _ error) error {
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

			upload(client, path)

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}

func upload(client jamsyncpb.JamsyncAPIClient, path string) {
	sourceBuffer, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}

	blockHashesPb, err := client.GetBlockHashes(context.Background(), &jamsyncpb.GetBlockHashesRequest{
		ProjectId: 1,
		BranchId:  1,
		Path:      path,
		Timestamp: timestamppb.Now(),
	})
	if err != nil {
		log.Fatal(err)
	}

	blockHashes := make([]rsync.BlockHash, len(blockHashesPb.GetBlockHashes()))
	for i, block := range blockHashesPb.GetBlockHashes() {
		blockHashes[i] = rsync.BlockHash{
			Index:      block.GetIndex(),
			StrongHash: block.GetStrongHash(),
			WeakHash:   block.GetWeakHash(),
		}
	}

	opsOut := make(chan rsync.Operation)
	rsDelta := &rsync.RSync{}
	go func() {
		var blockCt, blockRangeCt, dataCt, bytes int
		defer close(opsOut)
		err := rsDelta.CreateDelta(sourceBuffer, blockHashes, func(op rsync.Operation) error {
			switch op.Type {
			case rsync.OpBlockRange:
				blockRangeCt++
			case rsync.OpBlock:
				blockCt++
			case rsync.OpData:
				// Copy data buffer so it may be reused in internal buffer.
				b := make([]byte, len(op.Data))
				copy(b, op.Data)
				op.Data = b
				dataCt++
				bytes += len(op.Data)
			}
			opsOut <- op
			return nil
		})
		log.Printf("Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
		if err != nil {
			log.Fatalf("Failed to create delta: %s", err)
		}
	}()

	stream, err := client.UpdateStream(context.Background())
	if err != nil {
		panic(err)
	}

	for op := range opsOut {
		var opPbType jamsyncpb.OpType
		switch op.Type {
		case rsync.OpBlock:
			opPbType = jamsyncpb.OpType_OpBlock
		case rsync.OpData:
			opPbType = jamsyncpb.OpType_OpData
		case rsync.OpHash:
			opPbType = jamsyncpb.OpType_OpHash
		case rsync.OpBlockRange:
			opPbType = jamsyncpb.OpType_OpBlockRange
		}

		err := stream.Send(&jamsyncpb.UpdateStreamRequest{
			Operation: &jamsyncpb.Operation{
				OpType:        opPbType,
				BlockIndex:    op.BlockIndex,
				BlockIndexEnd: op.BlockIndexEnd,
				Data:          op.Data,
			},
			UserId:    1,
			ProjectId: 1,
			BranchId:  1,
			Path:      path,
		})
		if err != nil {
			log.Fatal(err)
		}
	}
	err = stream.CloseSend()
	if err != nil {
		log.Fatal(err)
	}
}
