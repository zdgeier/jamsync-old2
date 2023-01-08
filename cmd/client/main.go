package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/jamenv"
	jam "github.com/zdgeier/jamsync/internal/server/client"
	"github.com/zdgeier/jamsync/internal/server/clientauth"
	"github.com/zdgeier/jamsync/internal/server/server"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	err := jamenv.LoadFile()
	if err != nil {
		log.Panic(err)
	}

	accessToken, err := clientauth.InitConfig()
	if err != nil {
		log.Panic(err)
	}
	apiClient, closer, err := server.Connect(&oauth2.Token{
		AccessToken: accessToken,
	})
	if err != nil {
		log.Panic(err)
	}
	defer closer()

	currentPath, err := os.Getwd()
	if err != nil {
		log.Panic(err)
	}
	log.Println("Initializing a project at " + currentPath)

	empty, err := currentDirectoryEmpty()
	if err != nil {
		log.Panic(err)
	}

	var client *jam.Client
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

		client = jam.NewClient(apiClient, resp.ProjectId, resp.CurrentChange)
		err = downloadExistingProject(client)
		if err != nil {
			log.Panic(err)
		}
	} else if config := findJamsyncConfig(); config != nil {
		client = jam.NewClient(apiClient, config.ProjectId, config.CurrentChange)
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

		client = jam.NewClient(apiClient, resp.ProjectId, 0)
		err = uploadNewProject(client)
		if err != nil {
			log.Panic(err)
		}
	}

	fileMetadata := readLocalFileList()
	fileMetadataDiff, err := client.DiffLocalToRemote(context.Background(), fileMetadata)
	if err != nil {
		log.Panic(err)
	}
	err = pushFileListDiff(fileMetadata, fileMetadataDiff, client)
	if err != nil {
		log.Panic(err)
	}
}

func findJamsyncConfig() *pb.ProjectConfig {
	currentPath, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		filePath := fmt.Sprintf("%v/%v", currentPath, ".jamsync")
		fmt.Println("Looking for config in ", filePath)
		_, err := os.Stat(filePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			panic(err)
		} else if err == nil {
			if configBytes, err := os.ReadFile(filePath); err == nil {
				config := &pb.ProjectConfig{}
				err = proto.Unmarshal(configBytes, config)
				if err != nil {
					fmt.Println("Could not parse config file")
					return nil
				}
				return config
			}
		} else if currentPath == "/" {
			break
		}
		currentPath = path.Dir(currentPath)
	}
	return nil
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
	resp, err := client.DiffRemoteToLocal(context.Background(), &pb.FileMetadata{})
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
	ctx := context.Background()

	err := client.CreateChange()
	if err != nil {
		return err
	}

	log.Println("Uploading files...")
	for path, diff := range fileMetadataDiff.GetDiffs() {
		if diff.GetType() != pb.FileMetadataDiff_NoOp && diff.GetType() != pb.FileMetadataDiff_Delete && !diff.GetFile().GetDir() {
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
	err = client.CommitChange()
	if err != nil {
		return err
	}
	log.Println("Uploading file list...")

	err = client.UploadFileList(ctx, fileMetadata)
	if err != nil {
		return err
	}
	log.Println("Done")

	return nil
}

func applyFileListDiff(fileMetadataDiff *pb.FileMetadataDiff, client *jam.Client) error {
	ctx := context.Background()
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
	for w := 1; w <= 10; w++ {
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
