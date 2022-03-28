package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/cespare/xxhash"
	f "github.com/fauna/faunadb-go/v4/faunadb"
	"github.com/fsnotify/fsnotify"
)

type UserDirectory struct {
	Name  string `fauna:"name"`
	Owner f.RefV `fauna:"owner"`
}

type DirectoryVersion struct {
	Name          string            `fauna:"name"`
	PathVersions  map[string]string `fauna:"path_versions"`
	UserDirectory f.RefV            `fauna:"user_directory"`
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
	log.Println("Starting Jamsync!")
	client := f.NewFaunaClient(
		"fnAEis9V07ACUYR4UhhmybRX7C5ZR7jD-3QSfs-8",
		f.Endpoint("https://db.fauna.com"),
	)
	s3downloader := s3manager.NewDownloader(sess)
	s3uploader := s3manager.NewUploader(sess)

	currUserDirectoryRef := f.Ref(f.Collection("UserDirectories"), "327301565008314961")

	// Get current user directory info
	var userDirectory UserDirectory
	{
		res, err := client.Query(f.Get(currUserDirectoryRef))
		if err != nil {
			panic(err)
		}

		if err := res.At(f.ObjKey("data")).Get(&userDirectory); err != nil {
			panic(err)
		}
	}
	log.Println("Using UserDirectory", userDirectory.Name)

	// Start stream to DirectoryVersion

	var directoryVersion DirectoryVersion
	directoryVersionRef := f.Ref(f.Collection("DirectoryVersions"), "327302175592022609")
	subscription := client.Stream(directoryVersionRef)
	err := subscription.Start()
	if err != nil {
		panic(err)
	}

	for event := range subscription.StreamEvents() {
		log.Printf("Recieved event %+v\n", event.String())

		switch event.Type() {
		case f.StartEventT:
			// Get current version info
			{
				res, err := client.Query(f.Get(directoryVersionRef))
				if err != nil {
					panic(err)
				}
				if err := res.At(f.ObjKey("data")).Get(&directoryVersion); err != nil {
					panic(err)
				}
				log.Println("Using DirectoryVersion \"" + directoryVersion.Name + "\"")

				// Initialize directory with version
				syncDirectoryVersionLocally(s3downloader, directoryVersion)

				go watchAndUpdateDirectoryVersion(&directoryVersion, s3uploader, client, directoryVersionRef)
			}
		case f.HistoryRewriteEventT:
			// Do something with history rewrite events
		case f.VersionEventT:
			if versionEvent, ok := event.(f.VersionEvent); ok {
				if err := versionEvent.Event().At(f.ObjKey("document", "data")).Get(&directoryVersion); err != nil {
					panic(err)
				}

				syncDirectoryVersionLocally(s3downloader, directoryVersion)
			} else {
				panic("Could not type assert StartEvent")
			}
			// Do something with version events
		case f.ErrorEventT:
			// Handle stream errors
			// In this case, we close the stream
			log.Fatal(event)
			subscription.Close()
		}
	}
}

// Given a complete directory verion, update all the files in the current directory
// Does not download new files if hashes match
func syncDirectoryVersionLocally(s3downloader *s3manager.Downloader, directoryVersion DirectoryVersion) {
	for remotePath, remoteHash := range directoryVersion.PathVersions {
		if _, err := os.Stat(remotePath); errors.Is(err, os.ErrNotExist) {
			// Path does not exist, we can just download directly
			log.Println("Path does not exist downloading", remotePath)
			file, err := os.Create(remotePath)
			if err != nil {
				panic(err)
			}
			defer file.Close()

			_, err = s3downloader.Download(file,
				&s3.GetObjectInput{
					Bucket: aws.String("s3uploader-s3uploadbucket-8nvbzesahivf"),
					Key:    aws.String(fmt.Sprint(remoteHash)),
				})
			if err != nil {
				panic(err)
			}
			log.Println("Downloaded new file", remotePath)
		} else if err == nil {
			// Otherwise, we need to read the file and update if the hash has changed
			log.Println("An existing file is at the remote path", remotePath)
			contents, err := os.ReadFile(remotePath)
			if err != nil {
				panic(err)
			}
			digest := xxhash.New()
			digest.Write(contents)
			localHash := fmt.Sprint(digest.Sum64())

			if remoteHash != localHash {
				// Need to update local
				log.Println("Local and remote hashes do not match. Updating", remotePath, remoteHash)
				localFile, err := os.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE, os.ModePerm)
				if err != nil {
					panic(err)
				}
				// close fi on exit and check for its returned error
				defer func() {
					if err := localFile.Close(); err != nil {
						panic(err)
					}
				}()

				_, err = s3downloader.Download(localFile,
					&s3.GetObjectInput{
						Bucket: aws.String("s3uploader-s3uploadbucket-8nvbzesahivf"),
						Key:    aws.String(fmt.Sprint(remoteHash)),
					})
				if err != nil {
					panic(err)
				}
				log.Println("Updated file", remotePath)
			}
		} else {
			panic(err)
		}
	}
}

// Watches directory for changes.
// Uploads changed files and updates directory version once complete
func watchAndUpdateDirectoryVersion(directoryVersion *DirectoryVersion, s3uploader *s3manager.Uploader, client *f.FaunaClient, directoryVersionRef f.Expr) {
	// creates a new file watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	// starting at the root of the project, walk each file/directory searching for
	// directories
	if err := filepath.Walk(".", func(path string, fi os.FileInfo, err error) error {
		if !fi.Mode().IsDir() && !strings.Contains(path, ".git") {
			return watcher.Add(path)
		}

		return nil
	}); err != nil {
		fmt.Println("ERROR", err)
	}

	for {
		select {
		case event := <-watcher.Events:
			log.Println("Directory watcher event", event)
			if event.Name == ".jamsync" {
				continue
			}
			path := event.Name

			contents, err := ioutil.ReadFile(path)
			if err != nil {
				log.Fatal(err)
			}

			digest := xxhash.New()
			digest.Write(contents)
			localHash := fmt.Sprint(digest.Sum64())

			var sameLocalFile bool
			if remoteHash, ok := directoryVersion.PathVersions[path]; ok {
				// Path in versions, check hash and if different, upload
				if remoteHash == localHash {
					sameLocalFile = true
				}
			}

			if !sameLocalFile {
				// Hash not in versions or local hash is different than remote hash
				contentsReader := bytes.NewReader(contents)
				result, err := s3uploader.Upload(&s3manager.UploadInput{
					Bucket: aws.String("s3uploader-s3uploadbucket-8nvbzesahivf"),
					Key:    aws.String(fmt.Sprint(localHash)),
					Body:   contentsReader,
				})
				if err != nil {
					log.Fatalf("failed to upload file, %v", err)
				}
				log.Printf("Uploaded file to %s", aws.StringValue(&result.Location))

				_, err = client.Query(f.Update(directoryVersionRef, f.Obj{
					"data": f.Obj{
						"path_versions": f.Obj{
							path: localHash,
						},
					},
				}))
				if err != nil {
					panic(err)
				}

				log.Println("Updated directory version of", path)
			}
			//directoryVersion.setFileVersion(hash, path)

			//directoryVersion.WriteDirectoryVersionsFile()
		case err := <-watcher.Errors:
			log.Println("ERROR", err)
		}
	}
}
