package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func GetJamsyncFileInfo(client jamsyncpb.JamsyncAPIClient) (string, int, time.Time, error) {
	currentPath, err := os.Getwd()
	if err != nil {
		return "", -1, time.Time{}, err
	}

	for {
		fmt.Println(currentPath)
		// Simple file reading logic.
		filePath := fmt.Sprintf("%v/%v", currentPath, ".jamsync")
		_, err := os.Stat(filePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			panic(err)
		} else if err == nil {
			return parseJamsyncFile(filePath)
		} else if currentPath == "/" {
			break
		}
		currentPath = path.Dir(currentPath)
	}

	// No file found
	err = initializeJamsyncFile(client)
	if err != nil {
		log.Panic(err)
	}

	return parseJamsyncFile(".jamsync")
}

func parseJamsyncFile(path string) (string, int, time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", -1, time.Time{}, err
	}
	spl := strings.Split(string(data), " ")
	projectName := spl[0]
	changeId, err := strconv.Atoi(spl[1])
	if err != nil {
		return "", -1, time.Time{}, err
	}
	timeMillis, err := strconv.Atoi(spl[2])
	if err != nil {
		return "", -1, time.Time{}, err
	}
	return projectName, changeId, time.UnixMilli(int64(timeMillis)), nil
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
	resp, err := client.GetFile(context.Background(), &jamsyncpb.GetFileRequest{
		ProjectName: projectName,
		Path:        ".jamsyncfilelist",
		Timestamp:   timestamppb.Now(),
	})
	if err != nil {
		return err
	}

	var fileList *jamsyncpb.FileList
	err = proto.Unmarshal(resp.GetData(), fileList)
	if err != nil {
		return err
	}

	log.Println("Creating directories...")
	for _, file := range fileList.Files {
		if file.Dir {
			err = os.MkdirAll(file.GetPath(), os.ModePerm)
			if err != nil {
				return err
			}
		}
	}

	log.Println("Downloading files...")
	for _, file := range fileList.Files {
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

	log.Println("Done downloading.")
	return createJamsyncFile(projectName, currChangeResp.ChangeId, currChangeResp.Timestamp.AsTime())
}

func uploadNewProject(client jamsyncpb.JamsyncAPIClient, projectName string) error {
	log.Println("Adding existing files to project...")

	log.Println("Adding project...")
	_, err := client.AddProject(context.Background(), &jamsyncpb.AddProjectRequest{
		ProjectName: projectName,
	})
	if err != nil {
		return err
	}

	uploadLocalChanges(client, projectName)

	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(0))
	err = os.WriteFile(".jamsync", b, 0644)
	if err != nil {
		return err
	}

	log.Println("Done adding project.")
	return createJamsyncFile(projectName, 0, time.Now())
}

func readLocalFileList() *jamsyncpb.FileList {
	fileList := &jamsyncpb.FileList{
		Files: []*jamsyncpb.File{
			{
				Path: ".jamsyncfilelist",
				Dir:  false,
			},
		},
	}
	if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, _ error) error {
		if d.Name() == ".jamsync" || path == "." {
			return nil
		} else if d.IsDir() {
			fileList.Files = append(fileList.Files, &jamsyncpb.File{
				Path: path,
				Dir:  true,
			})
			return nil
		}
		fileList.Files = append(fileList.Files, &jamsyncpb.File{
			Path: path,
			Dir:  false,
		})
		return nil
	}); err != nil {
		log.Println("WARN: could not walk directory tree", err)
	}
	return fileList
}

func createJamsyncFile(projectName string, changeId uint64, timestamp time.Time) error {
	f, err := os.Create(".jamsync")
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(fmt.Sprintf("%s %d %d", projectName, changeId, timestamp.UTC().UnixMilli())) // writing...
	if err != nil {
		return err
	}
	return nil
}
