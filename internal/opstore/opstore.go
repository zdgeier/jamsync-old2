package opstore

import (
	"bytes"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/zdgeier/jamsync/internal/jamenv"
)

type OpStore interface {
	Read(projectId uint64, changeId uint64, pathHash uint64, offset uint64, length uint64) (data []byte, err error)
	Write(projectId uint64, changeId uint64, pathHash uint64, data []byte) (offset uint64, length uint64, err error)
}

func New() OpStore {
	var opStore OpStore
	switch jamenv.Env() {
	case jamenv.Prod:
		opStore = NewS3Store("jamsync-prod-us-east-1")
	case jamenv.Dev:
		opStore = NewS3Store("jamsync-dev-us-east-1")
	case jamenv.Local:
		opStore = NewLocalStore("jb")
	case jamenv.Memory:
		opStore = NewMemoryStore()
	}
	return opStore
}

type LocalStore struct {
	directory string
}

func NewLocalStore(directory string) LocalStore {
	return LocalStore{
		directory: directory,
	}
}

func (s LocalStore) changeDirectory(projectId uint64) string {
	return fmt.Sprintf("%s/%d/opdata", s.directory, projectId)
}
func (s LocalStore) filePath(projectId uint64, changeId uint64, pathHash uint64) string {
	return fmt.Sprintf("%s/%d.jb", s.changeDirectory(projectId), pathHash)
}
func (s LocalStore) Read(projectId uint64, changeId uint64, pathHash uint64, offset uint64, length uint64) (data []byte, err error) {
	currFile, err := os.OpenFile(s.filePath(projectId, changeId, pathHash), os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	b := make([]byte, length)
	_, err = currFile.ReadAt(b, int64(offset))
	if err != nil {
		return nil, err
	}
	return b, nil
}
func (s LocalStore) Write(projectId uint64, changeId uint64, pathHash uint64, data []byte) (offset uint64, length uint64, err error) {
	err = os.MkdirAll(s.changeDirectory(projectId), os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	currFile, err := os.OpenFile(s.filePath(projectId, changeId, pathHash), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}
	info, err := currFile.Stat()
	if err != nil {
		return 0, 0, err
	}
	writtenBytes, err := currFile.Write(data)
	if err != nil {
		return 0, 0, err
	}
	return uint64(info.Size()), uint64(writtenBytes), nil
}

type MemoryStore struct {
	files map[string][]byte
}

func NewMemoryStore() MemoryStore {
	return MemoryStore{
		files: make(map[string][]byte, 0),
	}
}
func (s MemoryStore) filePath(projectId uint64, changeId uint64, pathHash uint64) string {
	return fmt.Sprintf("%d/%d.jb", projectId, pathHash)
}
func (s MemoryStore) Read(projectId uint64, changeId uint64, pathHash uint64, offset uint64, length uint64) (data []byte, err error) {
	return s.files[s.filePath(projectId, changeId, pathHash)][offset : offset+length], nil
}
func (s MemoryStore) Write(projectId uint64, changeId uint64, pathHash uint64, data []byte) (offset uint64, length uint64, err error) {

	offset = uint64(len(s.files[s.filePath(projectId, changeId, pathHash)]))
	length = uint64(len(data))

	curr := append(s.files[s.filePath(projectId, changeId, pathHash)], data[:]...)
	s.files[s.filePath(projectId, changeId, pathHash)] = curr

	return offset, length, nil
}

type S3Store struct {
	bucketName string
	sess       *session.Session
	s3         *s3.S3
}

func NewS3Store(bucketName string) S3Store {
	sess := session.Must(session.NewSession())
	return S3Store{
		sess: sess,
		s3:   s3.New(sess),
	}
}
func (s S3Store) filePath(projectId uint64, changeId uint64, pathHash uint64) string {
	return fmt.Sprintf("%d/%d.jb", projectId, pathHash)
}
func (s S3Store) Read(projectId uint64, changeId uint64, pathHash uint64, offset uint64, length uint64) (data []byte, err error) {
	downloader := s3manager.NewDownloader(s.sess)
	buff := aws.NewWriteAtBuffer([]byte{})
	_, err = downloader.Download(buff, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(s.filePath(projectId, changeId, pathHash)),
	})
	if err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}
func (s S3Store) Write(projectId uint64, changeId uint64, pathHash uint64, data []byte) (offset uint64, length uint64, err error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(s.filePath(projectId, changeId, pathHash)),
	}
	info, err := s.s3.HeadObject(input)
	if err != nil {
		return 0, 0, err
	}
	offset = uint64(*info.ContentLength)
	uploader := s3manager.NewUploader(s.sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(s.filePath(projectId, changeId, pathHash)),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return 0, 0, err
	}
	return offset, uint64(len(data)), nil
}
