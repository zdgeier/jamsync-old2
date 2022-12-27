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
	"strings"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/pb"
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
	fileMetadataDiff, err := client.DiffLocalToRemote(context.Background(), fileMetadata)
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
		if d.Name() == ".jamsync" || path == "." || strings.HasPrefix(path, ".git") || strings.HasPrefix(path, "jb") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Println(path)

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

func downloadExistingProject(client *jam.Client) error {
	resp, err := client.DiffRemoteToLocal(context.TODO(), &pb.FileMetadata{})
	if err != nil {
		return err
	}

	err = applyFileListDiff(resp, client)
	if err != nil {
		return err
	}

	log.Println("Done downloading.")
	return writeJamsyncFile(client.ProjectConfig())
}

func pushFileListDiff(fileMetadata *pb.FileMetadata, fileMetadataDiff *pb.FileMetadataDiff, client *jam.Client) error {
	ctx := context.TODO()

	err := client.CreateChange()
	if err != nil {
		return err
	}

	log.Println("Uploading files...")
	for path, diff := range fileMetadataDiff.GetDiffs() {
		if diff.Type != pb.FileMetadataDiff_NoOp && !diff.File.Dir {
			//log.Println("Uploading " + path)
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
	client.CommitChange()
	log.Println("Uploading filelist...")

	return client.UploadFileList(ctx, fileMetadata)
}

func applyFileListDiff(fileMetadataDiff *pb.FileMetadataDiff, client *jam.Client) error {
	ctx := context.TODO()
	log.Println("Creating directories...")
	for path, diff := range fileMetadataDiff.GetDiffs() {
		if diff.GetType() != pb.FileMetadataDiff_NoOp && diff.GetFile().GetDir() {
			err := os.MkdirAll(path, os.ModePerm)
			if err != nil {
				return err
			}
		}
	}

	log.Println("Downloading files...")
	paths := make(chan string, len(fileMetadataDiff.GetDiffs()))
	results := make(chan error, len(fileMetadataDiff.GetDiffs()))

	worker := func(id int, paths <-chan string, results chan<- error) {
		for path := range paths {
			file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				results <- err
				return
			}

			fileContents, err := os.ReadFile(path)
			if err != nil {
				results <- err
				return
			}

			err = client.DownloadFile(ctx, path, bytes.NewReader(fileContents), file)
			if err != nil {
				results <- err
				return
			}

			results <- file.Close()
		}
	}

	// This starts up 3 workers, initially blocked
	// because there are no jobs yet.
	for w := 1; w <= 10000; w++ {
		go worker(w, paths, results)
	}
	for path, diff := range fileMetadataDiff.GetDiffs() {
		if diff.GetType() != pb.FileMetadataDiff_NoOp && !diff.GetFile().GetDir() {
			paths <- path
		}
	}
	close(paths)
	done := 0
	batchesDone := 0
	for _, diff := range fileMetadataDiff.GetDiffs() {
		if diff.GetType() != pb.FileMetadataDiff_NoOp && !diff.GetFile().GetDir() {
			<-results
			done += 1
			if done > 1000 {
				fmt.Println("Done: ", batchesDone*1000)
				batchesDone += 1
				done = 0
			}
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
