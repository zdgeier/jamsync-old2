package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/cespare/xxhash/v2"
	"github.com/fsnotify/fsnotify"
)

type FileVersionRow struct {
	Hash uint64
	Path string
}

var watcher *fsnotify.Watcher

func main() {
	fmt.Println("Starting!")
	// The session the S3 Uploader will use
	sess := session.Must(session.NewSession(
		&aws.Config{
			Region: aws.String("us-east-1"),
		},
	))

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	file, err := os.Open(".jamsync")
	if err != nil {
		log.Fatal(err)
	}

	versionFileScanner := bufio.NewScanner(file)
	var fileVersions []FileVersionRow
	for versionFileScanner.Scan() {
		scannedRow := strings.Split(versionFileScanner.Text(), "\t")

		hash, err := strconv.ParseUint(scannedRow[1], 10, 64)
		if err != nil {
			log.Fatal(err)
		}

		fileVersions = append(fileVersions, FileVersionRow{
			Path: scannedRow[0],
			Hash: uint64(hash),
		})
	}
	file.Close()

	var newFileVersions []FileVersionRow

	err = filepath.Walk(".",
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
				shouldUpload := true

				// Look up file in versions
				for _, v := range fileVersions {
					if v.Path == path && v.Hash == hash {
						// same path and hash, do nothin
						shouldUpload = false
						break
					} else if v.Path == path {
						// hash has changed, need to upload file with new hash
						break
					}
				}

				if shouldUpload {
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

				newFileVersions = append(newFileVersions, FileVersionRow{
					Path: path,
					Hash: hash,
				})
			}

			return nil
		})
	if err != nil {
		log.Fatal(err)
	}

	// Write new versions
	versionFileRef, err := os.OpenFile(".jamsync", os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	versionFile := bufio.NewWriter(versionFileRef)
	for _, v := range newFileVersions {
		_, err := versionFile.WriteString(v.Path + "\t" + fmt.Sprint(v.Hash) + "\n")
		if err != nil {
			log.Fatal(err)
		}
	}
	err = versionFile.Flush()
	if err != nil {
		log.Fatal(err)
	}

	// creates a new file watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	// starting at the root of the project, walk each file/directory searching for
	// directories
	if err := filepath.Walk(".", watchDir); err != nil {
		fmt.Println("ERROR", err)
	}

	//
	done := make(chan bool)

	//
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Name == ".jamsync" {
					continue
				}

				fmt.Printf("EVENT! %#v\n", event)
				versionFileRef, err := os.OpenFile(".jamsync", os.O_WRONLY|os.O_CREATE, os.ModePerm)
				if err != nil {
					log.Fatal(err)
				}
				versionFile := bufio.NewWriter(versionFileRef)
				for _, v := range newFileVersions {
					if v.Path == event.Name {
						contents, err := ioutil.ReadFile(v.Path)
						if err != nil {
							log.Fatal(err)
						}

						digest := xxhash.New()
						digest.Write(contents)
						hash := digest.Sum64()

						if hash != v.Hash {
							fmt.Println("uploading")
							contentsReader := bytes.NewReader(contents)
							result, err := uploader.Upload(&s3manager.UploadInput{
								Bucket: aws.String("s3uploader-s3uploadbucket-8nvbzesahivf"),
								Key:    aws.String(fmt.Sprint(hash)),
								Body:   contentsReader,
							})
							if err != nil {
								log.Fatal(err)
							}
							fmt.Printf("file uploaded to, %s\n", aws.StringValue(&result.Location))
						}
					}
					_, err := versionFile.WriteString(v.Path + "\t" + fmt.Sprint(v.Hash) + "\n")
					if err != nil {
						log.Fatal(err)
					}
				}
				err = versionFile.Flush()
				if err != nil {
					log.Fatal(err)
				}
				// watch for errors
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
	if fi.Mode().IsDir() && !strings.Contains(path, ".git") {
		return watcher.Add(path)
	}

	return nil
}
