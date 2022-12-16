package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/rsync"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var serverAddr = flag.String("addr", "18.216.248.73:14357", "The server address in the format of host:port")

type JamsyncProjectFile struct {
	ProjectName     string
	CurrentChangeId uint64
}

func main() {
	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Panicf("could not connect to jamsync server: %s", err)
	}
	defer conn.Close()

	client := jamsyncpb.NewJamsyncAPIClient(conn)

	projectName, localChangeId, timestamp, err := GetJamsyncFileInfo(client)
	if err != nil {
		log.Panic(err)
	}

	currChangeResp, err := client.GetCurrentChange(context.TODO(), &jamsyncpb.GetCurrentChangeRequest{
		ProjectName: projectName,
	})
	if err != nil {
		log.Panic(err)
	}
	remoteChangeId := int(currChangeResp.GetChangeId())

	if localChangeId < remoteChangeId {
		// Local version is behind remote version, we need to update but we also need to check for local changes
	} else if localChangeId == remoteChangeId {
		// We're up to date, remotely but we have to check for local changes...
	} else {
		panic("impossible...")
	}

	// starting at the root of the project, walk each file/directory searching for
	// directories
	changedFilePaths := make([]string, 0)
	if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, _ error) error {
		log.Println("Watching", path)
		if !d.IsDir() {
			fileInfo, err := d.Info()
			if err != nil {
				return err
			}

			if fileInfo.Name() == ".jamsync" {
				return nil
			}

			if fileInfo.ModTime().After(timestamp) {
				changedFilePaths = append(changedFilePaths, path)
			}
		}

		return nil
	}); err != nil {
		log.Panic("Could not walk directory tree to watch files", err)
	}

	log.Println("Changed:", changedFilePaths)
	if len(changedFilePaths) > 0 {
		uploadLocalChanges(client, projectName, changedFilePaths)
	}

	currChangeResp, err = client.GetCurrentChange(context.TODO(), &jamsyncpb.GetCurrentChangeRequest{
		ProjectName: projectName,
	})
	if err != nil {
		log.Panic("could not get current change")
	}
	err = createJamsyncFile(projectName, currChangeResp.ChangeId+1, currChangeResp.Timestamp.AsTime())
	if err != nil {
		log.Panic("could not update .jamsync")
	}

	log.Println("DONE")
}

func uploadLocalChanges(client jamsyncpb.JamsyncAPIClient, projectName string, paths []string) error {
	fileHashBlocksStream, err := client.GetFileHashBlocks(context.TODO(), &jamsyncpb.GetFileBlockHashesRequest{
		ProjectName: projectName,
		Paths:       paths,
	})
	if err != nil {
		return err
	}

	stream, err := client.ApplyOperations(context.Background())
	if err != nil {
		return err
	}
	for {
		blockHashesResp, err := fileHashBlocksStream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		blockHashesPb := blockHashesResp.GetBlockHashes()

		path := blockHashesResp.GetPath()
		sourceFile, err := os.Open(path)
		if err != nil {
			log.Fatal(err)
		}
		defer sourceFile.Close()

		sourceBytes, err := ioutil.ReadAll(sourceFile)
		if err != nil {
			log.Fatal(err)
		}

		blockHashes := make([]rsync.BlockHash, len(blockHashesPb))
		for i, block := range blockHashesPb {
			blockHashes[i] = rsync.BlockHash{
				Index:      block.GetIndex(),
				StrongHash: block.GetStrongHash(),
				WeakHash:   block.GetWeakHash(),
			}
		}

		opsOut := make(chan rsync.Operation)
		rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}
		go func() {
			sourceBuffer := bytes.NewReader(sourceBytes)
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

		allOps := make([]*jamsyncpb.Operation, 0)
		for op := range opsOut {
			var opPbType jamsyncpb.Operation_Type
			switch op.Type {
			case rsync.OpBlock:
				opPbType = jamsyncpb.Operation_OpBlock
			case rsync.OpData:
				opPbType = jamsyncpb.Operation_OpData
			case rsync.OpHash:
				opPbType = jamsyncpb.Operation_OpHash
			case rsync.OpBlockRange:
				opPbType = jamsyncpb.Operation_OpBlockRange
			}
			allOps = append(allOps, &jamsyncpb.Operation{
				Type:          opPbType,
				BlockIndex:    op.BlockIndex,
				BlockIndexEnd: op.BlockIndexEnd,
				Data:          op.Data,
			})
		}

		err = stream.Send(&jamsyncpb.ApplyOperationsRequest{
			Operations:  allOps,
			ProjectName: projectName,
			Path:        path,
		})
		if err != nil {
			log.Fatal(err)
		}
	}
	return stream.CloseSend()
}
