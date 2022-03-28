package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"


	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/cespare/xxhash/v2"
	"github.com/fsnotify/fsnotify"
)

type DirectoryVersion struct {
	hashToPaths map[uint64][]string
	pathToHash  map[string]uint64
}

func (dv DirectoryVersion) hasHashAndPath(hash uint64, path string) bool {
	for _, existingPasth := range dv.hashToPaths[hash] {
		if existingPath == path {
			return true
		}
	}
	return false
}

// Idempotently sets up hash and path maps
func (dv DirectoryVersion) setFileVersion(hash uint64, path string) {
	for _, existingPath := range dv.hashToPaths[hash] {
		if existingPath == path {
			// hash and path are the same, just return nothing
			return
		}
	}

	existingHash := dv.pathToHash[path]
	if existingHash == 0 {
		dv.hashToPaths[hash] = append(dv.hashToPaths[hash], path)
		dv.pathToHash[path] = hash
	} else {
		// Hash has changed for path, remove path from hash list
		for i, existingPath := range dv.hashToPaths[existingHash] {
			if existingPath == path {
				// remove
				arr := dv.hashToPaths[existingHash]
				arr[i] = arr[len(arr)-1]
				dv.hashToPaths[existingHash] = arr[:len(arr)-1]
				break
			}
		}

		dv.hashToPaths[hash] = append(dv.hashToPaths[hash], path)
		dv.pathToHash[path] = hash
	}
}

func (dv DirectoryVersion) WriteDirectoryVersionsFile() {
	// Write new versions
	versionFileRef, err := os.OpenFile(".jamsync", os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	versionFile := bufio.NewWriter(versionFileRef)
	for path, hash := range dv.pathToHash {
		_, err := versionFile.WriteString(path + "\t" + fmt.Sprint(hash) + "\n")
		if err != nil {
			log.Fatal(err)
		}
	}
	err = versionFile.Flush()
	if err != nil {
		log.Fatal(err)
	}
}

func ReadDirectoryVersionsFile() DirectoryVersion {
	file, err := os.Open(".jamsync")
	if err != nil {
		log.Fatal(err)
	}

	var directoryVersion DirectoryVersion
	directoryVersion.hashToPaths = make(map[uint64][]string)
	directoryVersion.pathToHash = make(map[string]uint64)

	versionFileScanner := bufio.NewScanner(file)
	for versionFileScanner.Scan() {
		scannedRow := strings.Split(versionFileScanner.Text(), "\t")

		hash, err := strconv.ParseUint(scannedRow[1], 10, 64)
		if err != nil {
			log.Fatal(err)
		}

		directoryVersion.setFileVersion(hash, scannedRow[0])
	}
	file.Close()

	return directoryVersion
}

var (
	watcher *fsnotify.Watcher
	sess    = session.Must(session.NewSession(
		&aws.Config{
			Region: aws.String("us-east-1"),
		},
	))
)

func main() {
	fmt.Println("Starting!")
	// The session the S3 Uploader will use

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	directoryVersion := ReadDirectoryVersionsFile()

	err := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if strings.Contains(path, ".git") || strings.Contains(path, ".jamsync") {
				return nil
			}

			if err != nil {
				log.Fatal(err)
			}

			if !info.IsDir() {
				fmt.Println(path, info.Size())

				contents, err := ioutil.ReadFile(path)
				if err != nil {
					return fmt.Errorf("failed to read file")
				}

				digest := xxhash.New()
				digest.Write(contents)
				hash := digest.Sum64()
				fmt.Println(digest.Sum64())

				if !directoryVersion.hasHashAndPath(hash, path) {
					contentsReader := bytes.NewReader(contents)
					result, err := uploader.Upload(&s3manager.UploadInput{
						Bucket: aws.String("s3uploader-s3uploadbucket-8nvbzesahivf"),
						Key:    aws.String(fmt.Sprint(hash)),
						Body:   contentsReader,
					})
					if err != nil {
						return fmt.Errorf("failed to upload file, %v", err)
					}
					fmt.Printf("file uploaded to, %s\n", aws.StringValue(&result.Location))
				}

				directoryVersion.setFileVersion(hash, path)
			}

			return nil
		})
	if err != nil {
		log.Fatal(err)
	}

	directoryVersion.WriteDirectoryVersionsFile()

	// creates a new file watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	// starting at the root of the project, walk each file/directory searching for
	// directories
	if err := filepath.Walk(".", watchDir); err != nil {
		fmt.Println("ERROR", err)
	}

	done := make(chan bool)

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Name == ".jamsync" {
					continue
				}

				fmt.Printf("EVENT! %#v\n", event)

				path := event.Name

				contents, err := ioutil.ReadFile(path)
				if err != nil {
					log.Fatal(err)
				}

				digest := xxhash.New()
				digest.Write(contents)
				hash := digest.Sum64()

				if !directoryVersion.hasHashAndPath(hash, path) {
					contentsReader := bytes.NewReader(contents)
					result, err := uploader.Upload(&s3manager.UploadInput{
						Bucket: aws.String("s3uploader-s3uploadbucket-8nvbzesahivf"),
						Key:    aws.String(fmt.Sprint(hash)),
						Body:   contentsReader,
					})
					if err != nil {
						log.Fatalf("failed to upload file, %v", err)
					}
					fmt.Printf("file uploaded to, %s\n", aws.StringValue(&result.Location))
				}
				directoryVersion.setFileVersion(hash, path)

				directoryVersion.WriteDirectoryVersionsFile()
			case err := <-watcher.Errors:
				fmt.Println("ERROR", err)
			}
		}
	}()

	<-done

	fmt.Println("Done")
}

// watchDir gets run as a walk func, searching for directories to add watchers to
func watchDir(path string, fi os.FileInfo, err error) error {

	// since fsnotify can watch all the files in a directory, watchers only need
	// to be added to each nested directory
	if !fi.Mode().IsDir() && !strings.Contains(path, ".git") {
		return watcher.Add(path)
	}

	return nil
}



	return nil
}

