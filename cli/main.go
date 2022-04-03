package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	mu            sync.Mutex
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
		"fnAEiuuCnaACUM3cHYLv2OSo_BYtymUbRor_eIR5",
		f.Endpoint("https://db.fauna.com"),
	)
	s3downloader := s3manager.NewDownloader(sess)
	s3uploader := s3manager.NewUploader(sess)

	currUserDirectoryRef := f.Ref(f.Collection("UserDirectories"), "327332730238927440")

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
	directoryVersionRef := f.Ref(f.Collection("DirectoryVersions"), "327332851032785488")
	subscription := client.Stream(directoryVersionRef)
	err := subscription.Start()
	if err != nil {
		panic(err)
	}
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
		case event := <-subscription.StreamEvents():
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
					syncDirectoryVersionLocally(s3downloader, &directoryVersion)
				}
			case f.HistoryRewriteEventT:
				// Do something with history rewrite events
			case f.VersionEventT:
				if versionEvent, ok := event.(f.VersionEvent); ok {
					if err := versionEvent.Event().At(f.ObjKey("document", "data")).Get(&directoryVersion); err != nil {
						panic(err)
					}

					directoryVersion.mu.Lock()
					syncDirectoryVersionLocally(s3downloader, &directoryVersion)
					directoryVersion.mu.Unlock()
				} else {
					panic("Could not type assert StartEvent")
				}
				// Do something with version events
			case f.ErrorEventT:
				// Handle stream errors
				log.Println(event)
			}
		case event := <-watcher.Events:
			directoryVersion.mu.Lock()
			log.Println("Directory watcher event", event)
			log.Println("HACK: waiting a second to prevent race condition for editor writing to file")
			time.Sleep(time.Second) // HACK: wait until file done being written to prevent race condition with editors
			if event.Name == ".jamsync" {
				continue
			}
			path := event.Name

			if pathStat, err := os.Stat(path); err == nil {
				fmt.Println(pathStat.Mode())
			} else {
				panic(err)
			}

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
			directoryVersion.mu.Unlock()
		case err := <-watcher.Errors:
			log.Println("ERROR", err)
		}
	}
}

// Given a complete directory verion, update all the files in the current directory
// Does not download new files if hashes match
func syncDirectoryVersionLocally(s3downloader *s3manager.Downloader, directoryVersion *DirectoryVersion) {
	for remotePath, remoteHash := range directoryVersion.PathVersions {
		if _, err := os.Stat(remotePath); errors.Is(err, os.ErrNotExist) {
			// Path does not exist, we can just download directly
			log.Println("Path does not exist downloading", remotePath)

			err := os.MkdirAll(filepath.Dir(remotePath), 0666)
			if err != nil {
				panic(err)
			}

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
				var outByteArr []byte
				var out *aws.WriteAtBuffer = aws.NewWriteAtBuffer(outByteArr)

				_, err = s3downloader.Download(out,
					&s3.GetObjectInput{
						Bucket: aws.String("s3uploader-s3uploadbucket-8nvbzesahivf"),
						Key:    aws.String(fmt.Sprint(remoteHash)),
					})
				if err != nil {
					panic(err)
				}
				err = os.WriteFile(remotePath, out.Bytes(), fs.FileMode(os.O_WRONLY))
				if err != nil {
					panic(err)
				}
				log.Println("Updated file", remotePath)
			} else {
				log.Println("Local and remote hashes match. Keeping the existing file.")
			}
		} else {
			panic(err)
		}
	}
}
