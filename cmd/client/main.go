package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/client"
	jam "github.com/zdgeier/jamsync/internal/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var serverAddr = flag.String("addr", "localhost:14357", "The server address in the format of host:port")

type JamsyncProjectFile struct {
	ProjectName     string
	CurrentChangeId uint64
}

func main() {
	// TODO Chroot
	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Panicf("could not connect to jamsync server: %s", err)
	}
	defer conn.Close()

	apiClient := pb.NewJamsyncAPIClient(conn)

	currentPath, err := os.Getwd()
	if err != nil {
		log.Panic(err)
	}
	log.Println("Initializing a project at " + currentPath)

	empty, err := currentDirectoryEmpty()
	if err != nil {
		log.Panic(err)
	}

	if empty {
		log.Println("This directory is empty.")
		log.Print("Name of project to download: ")
		var projectName string
		fmt.Scan(&projectName)

		resp, err := apiClient.GetProjectConfig(context.Background(), &pb.GetProjectConfigRequest{
			ProjectName: projectName,
		})
		if err != nil {
			log.Panic(err)
		}

		client := jam.NewClient(apiClient, resp.ProjectId, resp.CurrentChange)
		err = downloadExistingProject(client)
		if err != nil {
			log.Panic(err)
		}
	} else {
		log.Println("This directory has some existing contents.")
		log.Println("Name of new project to create for current directory: ")
		var projectName string
		fmt.Scan(&projectName)

		resp, err := apiClient.AddProject(context.Background(), &pb.AddProjectRequest{
			ProjectName: projectName,
		})
		if err != nil {
			log.Panic(err)
		}

		client := jam.NewClient(apiClient, resp.ProjectId, 0)
		err = uploadNewProject(client)
		if err != nil {
			log.Panic(err)
		}
	}
}

func writeJamsyncFile(config *pb.ProjectConfig) error {
	f, err := os.Create(".jamsync")
	if err != nil {
		return err
	}
	defer f.Close()

	configBytes, err := proto.Marshal(config)
	if err != nil {
		return err
	}
	_, err = f.Write(configBytes)
	return err
}

func uploadNewProject(client *jam.Client) error {
	fileMetadata := readLocalFileList()
	fileMetadataDiff, err := client.GetFileListDiff(context.Background(), fileMetadata)
	if err != nil {
		return err
	}
	err = pushFileListDiff(fileMetadata, fileMetadataDiff, client)
	if err != nil {
		return err
	}

	log.Println("Done adding project.")
	return writeJamsyncFile(client.ProjectConfig())
}

func readLocalFileList() *pb.FileMetadata {
	files := map[string]*pb.File{}
	if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, _ error) error {
		if d.Name() == ".jamsync" || path == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			files[path] = &pb.File{
				ModTime: timestamppb.New(info.ModTime()),
				Dir:     true,
			}
		} else {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h := xxhash.New()
			h.Write(data)

			files[path] = &pb.File{
				ModTime: timestamppb.New(info.ModTime()),
				Dir:     false,
				Hash:    h.Sum64(),
			}
		}
		return nil
	}); err != nil {
		log.Println("WARN: could not walk directory tree", err)
	}

	return &pb.FileMetadata{
		Files: files,
	}
}

func downloadExistingProject(client *client.Client) error {
	resp, err := client.GetFileListDiff(context.TODO(), &pb.FileMetadata{})
	if err != nil {
		return err
	}

	err = applyFileListDiff(resp, client)

	log.Println("Done downloading.")
	return writeJamsyncFile(client.ProjectConfig())
}

func pushFileListDiff(fileMetadata *pb.FileMetadata, fileMetadataDiff *pb.FileMetadataDiff, client *client.Client) error {
	ctx := context.TODO()

	err := client.CreateChange()
	if err != nil {
		return err
	}
	defer client.CommitChange()

	log.Println("Uploading files...")
	for path, diff := range fileMetadataDiff.GetDiffs() {
		if diff.Type != pb.FileMetadataDiff_NoOp && !diff.File.Dir {
			log.Println("Uploading " + path)
			file, err := os.OpenFile(path, os.O_RDONLY, 0755)
			if err != nil {
				return err
			}
			err = client.UploadFile(ctx, path, file)
			if err != nil {
				return err
			}
			file.Close()
		}
	}
	log.Println("Uploading filelist...")

	return client.UploadFileList(ctx, fileMetadata)
}

func applyFileListDiff(fileMetadataDiff *pb.FileMetadataDiff, client *client.Client) error {
	ctx := context.TODO()
	log.Println("Creating directories...")
	for path, diff := range fileMetadataDiff.GetDiffs() {
		if diff.Type != pb.FileMetadataDiff_NoOp && diff.File.Dir == true {
			err := os.MkdirAll(path, os.ModePerm)
			if err != nil {
				return err
			}
		}
	}

	log.Println("Downloading files...")
	for path, diff := range fileMetadataDiff.GetDiffs() {
		if diff.Type != pb.FileMetadataDiff_NoOp && diff.File.Dir == false {
			log.Println("Downloading " + path)

			file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				return err
			}

			err = client.DownloadFile(ctx, path, file, file)
			if err != nil {
				return err
			}

			file.Close()
		}
	}
	return nil
}

func currentDirectoryEmpty() (bool, error) {
	f, err := os.Open(".")
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}

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
