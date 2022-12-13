package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
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

	client := jamsyncpb.NewJamsyncAPIClient(conn)

	jamsyncFile, err := searchForJamsyncFile()
	if err != nil {
		log.Panic(err)
	}

	log.Println(jamsyncFile)
	if jamsyncFile == "" {
		err := initializeJamsyncFile(client)
		if err != nil {
			log.Panic(err)
		}
	}

	log.Println("DONE")
}

func searchForJamsyncFile() (string, error) {
	currentPath, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		fmt.Println(currentPath)
		// Simple file reading logic.
		filePath := fmt.Sprintf("%v/%v", currentPath, ".jamsync")
		_, err := os.Stat(filePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			panic(err)
		} else if err == nil {
			return filePath, nil
		} else if currentPath == "/" {
			break
		}
		currentPath = path.Dir(currentPath)
	}

	return "", nil
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

func initializeJamsyncFile(client jamsyncpb.JamsyncAPIClient) error {
	currentPath, err := os.Getwd()
	if err != nil {
		return err
	}
	log.Println("Initializing a project at " + currentPath)

	empty, err := currentDirectoryEmpty()
	if err != nil {
		return err
	}

	if empty {
		log.Println("This directory is empty.")
		log.Print("Name of project to download: ")
		var projectName string
		fmt.Scan(&projectName)
		return downloadExistingProject(client, projectName)
	} else {
		log.Println("This directory has some existing contents.")
		log.Println("Name of new project to create for current directory: ")
		var projectName string
		fmt.Scan(&projectName)
		return uploadNewProject(client, projectName)
	}
}

func downloadExistingProject(client jamsyncpb.JamsyncAPIClient, projectName string) error {
	resp, err := client.GetFileList(context.TODO(), &jamsyncpb.GetFileListRequest{
		ProjectName: projectName,
	})
	if err != nil {
		return err
	}

	log.Println("Creating directories...")
	for _, file := range resp.Files {
		if file.Dir {
			err = os.MkdirAll(file.GetPath(), os.ModePerm)
			if err != nil {
				return err
			}
		}
	}

	log.Println("Downloading files...")
	for _, file := range resp.Files {
		if !file.Dir {
			log.Println("Downloading " + file.GetPath())
			resp, err := client.GetFile(context.TODO(), &jamsyncpb.GetFileRequest{
				ProjectName: projectName,
				Path:        file.GetPath(),
			})
			if err != nil {
				return err
			}

			// TODO: filemode
			if err := os.WriteFile(file.GetPath(), resp.GetData(), 0644); err != nil {
				return err
			}
		}
	}

	currChangeResp, err := client.GetCurrentChange(context.TODO(), &jamsyncpb.GetCurrentChangeRequest{ProjectName: projectName})
	if err != nil {
		return err
	}

	createJamsyncFile(currChangeResp.GetChangeId())

	log.Println("Done downloading.")
	return nil
}

func uploadNewProject(client jamsyncpb.JamsyncAPIClient, projectName string) error {
	existingFiles := make([]*jamsyncpb.File, 0)
	existingData := make([][]byte, 0)

	log.Println("Adding existing files to project...")
	if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, _ error) error {
		if d.Name() == ".jamsync" || path == "." {
			return nil
		} else if d.IsDir() {
			existingFiles = append(existingFiles, &jamsyncpb.File{
				Path: path,
				Dir:  true,
			})
			existingData = append(existingData, nil)
			return nil
		}
		existingFiles = append(existingFiles, &jamsyncpb.File{
			Path: path,
			Dir:  false,
		})
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		existingData = append(existingData, data)
		return nil
	}); err != nil {
		log.Println("WARN: could not walk directory tree", err)
	}

	log.Println("Adding project...")
	_, err := client.AddProject(context.Background(), &jamsyncpb.AddProjectRequest{
		ProjectName: projectName,
		ExistingFiles: &jamsyncpb.GetFileListResponse{
			Files: existingFiles,
		},
		ExistingData: existingData,
	})
	if err != nil {
		return err
	}

	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(0))
	err = os.WriteFile(".jamsync", b, 0644)
	if err != nil {
		return err
	}

	createJamsyncFile(1)

	log.Println("Done adding project.")
	return nil
}

func createJamsyncFile(changeId uint64) error {
	f, err := os.Create(".jamsync")
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(strconv.Itoa(int(changeId))) // writing...
	if err != nil {
		return err
	}
	return nil
}
