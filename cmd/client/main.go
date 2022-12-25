package main

import (
	"flag"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var serverAddr = flag.String("addr", "localhost:14357", "The server address in the format of host:port")

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

	// client := pb.NewJamsyncAPIClient(conn)
	// currFileMetadata := &pb.FileMetadata{}
	// if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, _ error) error {
	// 	fileInfo, err := d.Info()
	// 	if err != nil {
	// 		return err
	// 	}
	// 	dir := d.IsDir()
	// 	currFileMetadata.Files[path] = &pb.File{
	// 		Dir:     dir,
	// 		ModTime: timestamppb.New(fileInfo.ModTime()),
	// 	}
	// 	return nil
	// }); err != nil {
	// 	log.Panic("Could not walk directory tree to watch files", err)
	// }

	// projectName, localChangeId, timestamp, err := GetJamsyncFileInfo(client)
	// if err != nil {
	// 	log.Panic(err)
	// }

	// currChangeResp, err := client.GetCurrentChange(context.TODO(), &pb.GetCurrentChangeRequest{
	// 	ProjectName: projectName,
	// })
	// if err != nil {
	// 	log.Panic(err)
	// }
	// remoteChangeId := int(currChangeResp.GetChangeId())

	// if localChangeId < remoteChangeId {
	// 	// Local version is behind remote version, we need to update but we also need to check for local changes
	// } else if localChangeId == remoteChangeId {
	// 	// We're up to date, remotely but we have to check for local changes...
	// } else {
	// 	panic("impossible...")
	// }

	// // starting at the root of the project, walk each file/directory searching for
	// // directories
	// changedFilePaths := make([]string, 0)
	// if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, _ error) error {
	// 	log.Println("Watching", path)
	// 	if !d.IsDir() {
	// 		fileInfo, err := d.Info()
	// 		if err != nil {
	// 			return err
	// 		}

	// 		if fileInfo.Name() == ".jamsync" {
	// 			return nil
	// 		}

	// 		if fileInfo.ModTime().After(timestamp) {
	// 			changedFilePaths = append(changedFilePaths, path)
	// 		}
	// 	}

	// 	return nil
	// }); err != nil {
	// 	log.Panic("Could not walk directory tree to watch files", err)
	// }

	// log.Println("Changed:", changedFilePaths)
	// if len(changedFilePaths) > 0 {
	// 	err := uploadLocalChanges(client, projectName)
	// 	if err != nil {
	// 		log.Panic(err)
	// 	}
	// }

	// currChangeResp, err = client.GetCurrentChange(context.TODO(), &pb.GetCurrentChangeRequest{
	// 	ProjectName: projectName,
	// })
	// if err != nil {
	// 	log.Panic("could not get current change")
	// }
	// err = createJamsyncFile(projectName, currChangeResp.ChangeId, currChangeResp.Timestamp.AsTime())
	// if err != nil {
	// 	log.Panic("could not update .jamsync")
	// }

	// log.Println("DONE")
}

// func uploadLocalChanges(client pb.JamsyncAPIClient, projectName string) error {
// 	log.Println("uploading")
// 	localFileList := readLocalFileList()
// 	fileHashBlocksStream, err := client.GetFileHashBlocks(context.TODO(), &pb.GetFileBlockHashesRequest{
// 		ProjectName: projectName,
// 		FileList:    localFileList,
// 	})
// 	if err != nil {
// 		return err
// 	}
//
// 	log.Println("stream1")
// 	changeRequest, err := client.CreateChange(context.TODO(), &pb.CreateChangeRequest{
// 		ProjectName: projectName,
// 	})
// 	if err != nil {
// 		return err
// 	}
//
// 	stream, err := client.StreamChange(context.Background())
// 	if err != nil {
// 		return err
// 	}
// 	log.Println("loopin")
// 	for {
// 		blockHashesResp, err := fileHashBlocksStream.Recv()
// 		if err == io.EOF {
// 			break
// 		}
// 		if err != nil {
// 			return err
// 		}
// 		blockHashesPb := blockHashesResp.GetBlockHashes()
//
// 		log.Println("gotem")
// 		var sourceBytes []byte
// 		path := blockHashesResp.GetPath()
// 		if path == ".jamsyncfilelist" {
// 			sourceBytes, err = proto.Marshal(localFileList)
// 			if err != nil {
// 				log.Panic(err)
// 			}
// 		} else {
// 			sourceFile, err := os.Open(path)
// 			if err != nil {
// 				log.Panic(err)
// 			}
// 			defer sourceFile.Close()
//
// 			sourceBytes, err = ioutil.ReadAll(sourceFile)
// 			if err != nil {
// 				log.Panic(err)
// 			}
// 		}
//
// 		blockHashes := make([]rsync.BlockHash, len(blockHashesPb))
// 		for i, block := range blockHashesPb {
// 			blockHashes[i] = rsync.BlockHash{
// 				Index:      block.GetIndex(),
// 				StrongHash: block.GetStrongHash(),
// 				WeakHash:   block.GetWeakHash(),
// 			}
// 		}
//
// 		log.Println("making bacon")
// 		opsOut := make(chan rsync.Operation)
// 		rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}
// 		go func() {
// 			sourceBuffer := bytes.NewReader(sourceBytes)
// 			var blockCt, blockRangeCt, dataCt, bytes int
// 			defer close(opsOut)
// 			err := rsDelta.CreateDelta(sourceBuffer, blockHashes, func(op rsync.Operation) error {
// 				switch op.Type {
// 				case rsync.OpBlockRange:
// 					blockRangeCt++
// 				case rsync.OpBlock:
// 					blockCt++
// 				case rsync.OpData:
// 					// Copy data buffer so it may be reused in internal buffer.
// 					b := make([]byte, len(op.Data))
// 					copy(b, op.Data)
// 					op.Data = b
// 					dataCt++
// 					bytes += len(op.Data)
// 				}
// 				opsOut <- op
// 				return nil
// 			})
// 			log.Printf("Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dKiB", blockRangeCt, blockCt, dataCt, bytes/1024)
// 			if err != nil {
// 				log.Panicf("Failed to create delta: %s", err)
// 			}
// 		}()
//
// 		log.Println("making bacons")
// 		for op := range opsOut {
// 			log.Println("making opss")
// 			var opPbType pb.Operation_Type
// 			switch op.Type {
// 			case rsync.OpBlock:
// 				opPbType = pb.Operation_OpBlock
// 			case rsync.OpData:
// 				opPbType = pb.Operation_OpData
// 			case rsync.OpHash:
// 				opPbType = pb.Operation_OpHash
// 			case rsync.OpBlockRange:
// 				opPbType = pb.Operation_OpBlockRange
// 			}
//
// 			err = stream.Send(&pb.ChangeOperation{
// 				ProjectId: changeRequest.GetProjectId(),
// 				ChangeId:  changeRequest.GetChangeId(),
// 				PathHash:  pathToHash(path),
// 				Op: &pb.Operation{
// 					Type:          opPbType,
// 					BlockIndex:    op.BlockIndex,
// 					BlockIndexEnd: op.BlockIndexEnd,
// 					Data:          op.Data,
// 				},
// 			})
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}
// 	log.Println("done")
// 	return stream.CloseSend()
// }
//
// func pathToHash(path string) uint64 {
// 	h := xxhash.New()
// 	h.Write([]byte(path))
// 	return h.Sum64()
// }
//
