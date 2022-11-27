package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/cespare/xxhash/v2"
	"github.com/pierrec/lz4/v4"
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
		log.Panicf("fail to dial: %v", err)
	}
	defer conn.Close()
	client := jamsyncpb.NewJamsyncAPIClient(conn)

	// ZUCC: Below can be optimized by looking at the time modification tag and checking if it's been changed since the last run
	// Also spawning multiple goroutines to do this would be nice.
	hasher := xxhash.New()
	paths := make([]*jamsyncpb.PathData, 0)
	if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, _ error) error {
		if d.IsDir() {
			paths = append(paths, &jamsyncpb.PathData{
				Path: path,
			})
		} else {
			contents, err := os.ReadFile(path)
			if err != nil {
				// TODO: This is erroring
				return nil
			}
			hasher.Write(contents)
			paths = append(paths, &jamsyncpb.PathData{
				Path: path,
				Hash: hasher.Sum64(),
			})

			hasher.Reset()
		}
		return nil
	}); err != nil {
		log.Panic("Could not walk directory tree to watch files", err)
	}

	pathMetadata := jamsyncpb.PathsMetadata{
		Paths: paths,
	}

	upload(client, []byte(pathMetadata.String()), "jamsync.pm")
}

func upload(client jamsyncpb.JamsyncAPIClient, sourceBytes []byte, path string) {
	fmt.Println("start")
	blockHashesPb, err := client.GetBlockHashes(context.Background(), &jamsyncpb.GetBlockHashesRequest{
		ProjectId: 1,
		BranchId:  1,
		Path:      path,
		Timestamp: timestamppb.Now(),
	})
	if err != nil {
		log.Panic(err)
	}
	fmt.Println("end")

	blockHashes := make([]rsync.BlockHash, len(blockHashesPb.GetBlockHashes()))
	for i, block := range blockHashesPb.GetBlockHashes() {
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
			log.Panicf("Failed to create delta: %s", err)
		}
	}()

	stream, err := client.UpdateStream(context.Background())
	if err != nil {
		panic(err)
	}

	hasher := xxhash.New()
	_, err = hasher.Write(sourceBytes)
	if err != nil {
		log.Panic(err)
	}
	hash := hasher.Sum64()

	i := 0
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
		r := bytes.NewReader(op.Data)

		zout := new(bytes.Buffer)
		zw := lz4.NewWriter(zout)

		_, err = io.Copy(zw, r)
		if err != nil {
			log.Fatal(err)
		}
		err = zw.Close()
		if err != nil {
			log.Fatal(err)
		}

		if i == 0 {
			err := stream.Send(&jamsyncpb.UpdateStreamRequest{
				Operation: &jamsyncpb.Operation{
					OpType:        opPbType,
					BlockIndex:    op.BlockIndex,
					BlockIndexEnd: op.BlockIndexEnd,
					Data:          zout.Bytes(),
				},
				ProjectId: 1,
				BranchId:  1,
				PathData: &jamsyncpb.PathData{
					Path: path,
					Hash: hash,
				},
			})
			if err != nil {
				log.Panic(err)
			}
		} else {
			err := stream.Send(&jamsyncpb.UpdateStreamRequest{
				Operation: &jamsyncpb.Operation{
					OpType:        opPbType,
					BlockIndex:    op.BlockIndex,
					BlockIndexEnd: op.BlockIndexEnd,
					Data:          op.Data,
				},
				ProjectId: 1,
				BranchId:  1,
			})
			if err != nil {
				log.Panic(err)
			}
		}

		i += 1
	}
	err = stream.CloseSend()
	if err != nil {
		log.Panic(err)
	}
}

// func upload(client jamsyncpb.JamsyncAPIClient, path string) {
// 	sourceFile, err := os.Open(path)
// 	if err != nil {
// 		log.Panic(err)
// 	}
// 	defer sourceFile.Close()
//
// 	sourceBytes, err := ioutil.ReadAll(sourceFile)
// 	if err != nil {
// 		log.Panic(err)
// 	}
//
// 	fmt.Println("start")
// 	blockHashesPb, err := client.GetBlockHashes(context.Background(), &jamsyncpb.GetBlockHashesRequest{
// 		ProjectId: 1,
// 		BranchId:  1,
// 		Path:      path,
// 		Timestamp: timestamppb.Now(),
// 	})
// 	if err != nil {
// 		log.Panic(err)
// 	}
// 	fmt.Println("end")
//
// 	blockHashes := make([]rsync.BlockHash, len(blockHashesPb.GetBlockHashes()))
// 	for i, block := range blockHashesPb.GetBlockHashes() {
// 		blockHashes[i] = rsync.BlockHash{
// 			Index:      block.GetIndex(),
// 			StrongHash: block.GetStrongHash(),
// 			WeakHash:   block.GetWeakHash(),
// 		}
// 	}
//
// 	opsOut := make(chan rsync.Operation)
// 	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}
// 	go func() {
// 		sourceBuffer := bytes.NewReader(sourceBytes)
// 		var blockCt, blockRangeCt, dataCt, bytes int
// 		defer close(opsOut)
// 		err := rsDelta.CreateDelta(sourceBuffer, blockHashes, func(op rsync.Operation) error {
// 			switch op.Type {
// 			case rsync.OpBlockRange:
// 				blockRangeCt++
// 			case rsync.OpBlock:
// 				blockCt++
// 			case rsync.OpData:
// 				// Copy data buffer so it may be reused in internal buffer.
// 				b := make([]byte, len(op.Data))
// 				copy(b, op.Data)
// 				op.Data = b
// 				dataCt++
// 				bytes += len(op.Data)
// 			}
// 			opsOut <- op
// 			return nil
// 		})
// 		log.Printf("Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
// 		if err != nil {
// 			log.Panicf("Failed to create delta: %s", err)
// 		}
// 	}()
//
// 	stream, err := client.UpdateStream(context.Background())
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	i := 0
// 	for op := range opsOut {
// 		var opPbType jamsyncpb.OpType
// 		switch op.Type {
// 		case rsync.OpBlock:
// 			opPbType = jamsyncpb.OpType_OpBlock
// 		case rsync.OpData:
// 			opPbType = jamsyncpb.OpType_OpData
// 		case rsync.OpHash:
// 			opPbType = jamsyncpb.OpType_OpHash
// 		case rsync.OpBlockRange:
// 			opPbType = jamsyncpb.OpType_OpBlockRange
// 		}
//
// 		if i == 0 {
// 			err := stream.Send(&jamsyncpb.UpdateStreamRequest{
// 				Operation: &jamsyncpb.Operation{
// 					OpType:        opPbType,
// 					BlockIndex:    op.BlockIndex,
// 					BlockIndexEnd: op.BlockIndexEnd,
// 					Data:          op.Data,
// 				},
// 				UserId:    1,
// 				ProjectId: 1,
// 				BranchId:  1,
// 				PathData: &jamsyncpb.UpdateStreamRequest_PathData{
// 					Path: path,
// 					Dir:  false,
// 				},
// 			})
// 			if err != nil {
// 				log.Panic(err)
// 			}
// 		} else {
// 			err := stream.Send(&jamsyncpb.UpdateStreamRequest{
// 				Operation: &jamsyncpb.Operation{
// 					OpType:        opPbType,
// 					BlockIndex:    op.BlockIndex,
// 					BlockIndexEnd: op.BlockIndexEnd,
// 					Data:          op.Data,
// 				},
// 				UserId:    1,
// 				ProjectId: 1,
// 				BranchId:  1,
// 			})
// 			if err != nil {
// 				log.Panic(err)
// 			}
// 		}
//
// 		i += 1
// 	}
// 	err = stream.CloseSend()
// 	if err != nil {
// 		log.Panic(err)
// 	}
// }
