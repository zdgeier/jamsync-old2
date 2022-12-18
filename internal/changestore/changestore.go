package changestore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"github.com/zdgeier/jamsync/internal/rsync"
	"google.golang.org/protobuf/proto"
)

const (
	localChangeDirectory = "jamsyncdata"
)

type ChangeStore interface {
	WriteFileList(projectName string, fileList jamsyncpb.GetFileListResponse) (int64, int, error)
	ReadFileList(projectName string) (int64, int, error)
	WriteFile(projectName string, path string, data []byte) (int64, int, error)
	WriteChangeData(projectName string, path string, changeData *jamsyncpb.ChangeData) (int64, int, error)
	ReadChangeData(projectName string, path string, changeLocations []ChangeLocation) ([]*jamsyncpb.ChangeData, error)
}

type LocalChangeStore struct {
}

func getLocalProjectDirectory(projectName string) string {
	return fmt.Sprintf(localChangeDirectory + "/" + projectName)
}

func (s LocalChangeStore) ReadFileList(projectName string, timestamp time.Time) (*bytes.Reader, error) {
	log.Println("GetFileList")
	targetBuffer, err := RegenFile(s, projectName, "jamsyncfilelist", timestamp)
	if err != nil {
		return nil, err
	}
	return RegenFile(s, projectName, ".jamsyncfilelist", timestamp)
}

func (s LocalChangeStore) WriteFileList(projectName string, fileList *jamsyncpb.GetFileListResponse) (int64, int, error) {
	fileListData, err := proto.Marshal(fileList)
	if err != nil {
		return 0, 0, err
	}
	return s.WriteFile(projectName, ".jamsyncfilelist", fileListData)
}

func (s LocalChangeStore) WriteFile(projectName string, path string, data []byte) (int64, int, error) {
	ops := generateRsyncOpsForNewFile(data)

	opsPb := make([]*jamsyncpb.Operation, 0)
	for _, op := range ops {
		opPb := rsync.RsyncOperationToPb(&op)
		opsPb = append(opsPb, &opPb)
	}
	return s.WriteChangeData(projectName, path, &jamsyncpb.ChangeData{
		Ops: opsPb,
	})
}

func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

	sourceBuffer := bytes.NewReader(data)

	opsOut := make([]rsync.Operation, 0)
	var blockCt, blockRangeCt, dataCt, bytes int
	err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
		switch op.Type {
		case rsync.OpBlockRange:
			blockRangeCt++
		case rsync.OpBlock:
			blockCt++
		case rsync.OpData:
			// Copy data buffer so it may be reused in internal buffer.
			b := make([]byte, len(op.Data))
			copy(b, op.Data)
			op.Data = b
			dataCt++
			bytes += len(op.Data)
		}
		opsOut = append(opsOut, op)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to create delta: %s", err)
	}

	return opsOut
}

func dataFilePath(projectDir string, path string) string {
	dir := getLocalProjectDirectory(projectDir)
	return fmt.Sprintf("%s/%s.jb", dir, base64.StdEncoding.EncodeToString([]byte(path)))
}

func (s LocalChangeStore) WriteChangeData(projectName string, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
	dir := getLocalProjectDirectory(projectName)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return 0, 0, err
	}

	f, err := os.OpenFile(dataFilePath(dir, path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	bytes, err := proto.Marshal(changeData)
	if err != nil {
		return 0, 0, err
	}

	n, err := f.Write(bytes)
	if err != nil {
		log.Fatal(err)
		return 0, 0, err
	}

	return info.Size(), n, nil
}

type ChangeLocation struct {
	offset int
	length int
}

func (s LocalChangeStore) ReadChangeData(projectName string, path string, changeLocations []ChangeLocation) ([]*jamsyncpb.ChangeData, error) {
	f, err := os.Open(dataFilePath(getLocalProjectDirectory(projectName), path))
	if err != nil {
		return nil, err
	}

	changeDatas := make([]*jamsyncpb.ChangeData, 0)
	for _, changeLocation := range changeLocations {
		changeFile := make([]byte, changeLocation.length)
		n, err := f.ReadAt(changeFile, int64(changeLocation.offset))
		if err != nil {
			return nil, err
		}
		if n != int(changeLocation.length) {
			return nil, errors.New("read length does not equal expected")
		}

		change := &jamsyncpb.ChangeData{}
		err = proto.Unmarshal(changeFile, change)
		if err != nil {
			return nil, err
		}
		changeDatas = append(changeDatas, change)
	}
	return changeDatas, nil
}

func RegenFile(s ChangeStore, projectName string, path string, changeLocations []ChangeLocation) (*bytes.Reader, error) {
	changeDatas, err := s.ReadChangeData(projectName, path, changeLocations)
	if err != nil {
		return nil, err
	}

	changeOps := make([][]rsync.Operation, 0, len(changeDatas))
	for _, change := range changeDatas {
		opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

		for _, op := range change.GetOps() {
			opsOut = append(opsOut, rsync.PbOperationToRsync(op))
		}
		changeOps = append(changeOps, opsOut)
	}

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	targetBuffer := bytes.NewReader([]byte{})
	result := new(bytes.Buffer)
	for _, ops := range changeOps {
		err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
		if err != nil {
			return nil, err
		}
		resBytes := result.Bytes()
		targetBuffer = bytes.NewReader(resBytes)
		result.Reset()
	}

	return targetBuffer, nil
}
