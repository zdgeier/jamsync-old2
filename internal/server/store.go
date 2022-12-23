package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/zdgeier/jamsync/gen/jamsyncpb"
	"google.golang.org/protobuf/proto"
)

type JamsyncStore struct {
	jamsyncpb.UnimplementedJamsyncStoreServer
}

func NewStore() JamsyncStore {
	server := JamsyncStore{}
	return server
}

/*
rpc ReadChangeData(stream ChangeLocation) returns (stream jamsyncpb.Operation);

    rpc StartChange(StartChangeRequest) returns (StartChangeResponse);
    rpc WriteChangeDatas(stream ChangeOperation) returns (stream ChangeLocation);
    rpc CommitChange(CommitChangeRequest) returns (CommitChangeResponse);
*/

const (
	localChangeDirectory = "jamsyncdata"
)

func changeDirectory(projectId uint64, changeId uint64) string {
	return fmt.Sprintf("%s/%d/%d", localChangeDirectory, projectId, changeId)
}
func filePath(projectId uint64, changeId uint64, pathHash uint64) string {
	return fmt.Sprintf("%s/%d.jb", changeDirectory(projectId, changeId), pathHash)
}

func (s JamsyncStore) ReadChangeData(in *jamsyncpb.ChangeLocationList, srv jamsyncpb.JamsyncStore_ReadChangeDataServer) error {
	currFile, err := os.OpenFile(filePath(in.ProjectId, in.ChangeId, in.PathHash), os.O_RDONLY, 0644)
	if err != nil {
		fmt.Println("DOn5")
		return err
	}

	for _, loc := range in.ChangeLocations {
		fmt.Println("reading", loc.Offset, loc.Length)
		b := make([]byte, loc.Length)
		_, err = currFile.ReadAt(b, int64(loc.Offset))
		if err != nil {
			fmt.Println("DOn4")
			fmt.Println(err)
			return err
		}

		op := new(jamsyncpb.Operation)
		fmt.Println(b)
		err = proto.Unmarshal(b, op)
		if err != nil {
			fmt.Println("DOn3")
			return err
		}

		err = srv.Send(op)
		if err != nil {
			fmt.Println("DOn2")
			return err
		}
	}

	return nil
}
func (s JamsyncStore) CreateChangeDir(ctx context.Context, in *jamsyncpb.CreateChangeDirRequest) (*jamsyncpb.CreateChangeDirResponse, error) {
	// TODO: track change dir int
	dir := changeDirectory(in.GetProjectId(), in.GetChangeId())
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return nil, err
	}
	return &jamsyncpb.CreateChangeDirResponse{}, nil
}
func (s JamsyncStore) StreamChange(srv jamsyncpb.JamsyncStore_StreamChangeServer) error {
	log.Println("StoreStream")
	currChangeId := uint64(0)
	currProjectId := uint64(0)
	currPathHash := uint64(0)
	currOffset := uint64(0)
	currLength := uint64(0)
	changeLocations := make([]*jamsyncpb.ChangeLocation, 0)
	currFile := new(os.File)
	for {
		in, err := srv.Recv()
		if err == io.EOF {
			fmt.Println("STORE", changeLocations)
			err := srv.Send(&jamsyncpb.ChangeLocationList{
				ChangeId:        currChangeId,
				ProjectId:       currProjectId,
				ChangeLocations: changeLocations,
				PathHash:        currPathHash,
			})
			if err != nil {
				return err
			}
			break
		}
		if err != nil {
			return err
		}

		if in.GetPathHash() != currPathHash {
			if currPathHash != 0 {
				// We've completed a path hash writting
				fmt.Println("STORE0", changeLocations)
				err := srv.Send(&jamsyncpb.ChangeLocationList{
					ChangeId:        currChangeId,
					ProjectId:       currProjectId,
					ChangeLocations: changeLocations,
					PathHash:        currPathHash,
				})
				if err != nil {
					return err
				}
			}

			f, err := os.OpenFile(filePath(in.ProjectId, in.ChangeId, in.PathHash), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			currFile = f

			info, err := currFile.Stat()
			if err != nil {
				return err
			}
			currOffset = uint64(info.Size())
			currLength = 0
			currPathHash = in.GetPathHash()
			currChangeId = in.GetChangeId()
			currProjectId = in.GetProjectId()
			changeLocations = make([]*jamsyncpb.ChangeLocation, 0)
		}

		opBytes, err := proto.Marshal(in.Op)
		if err != nil {
			return err
		}
		writtenBytes, err := currFile.Write(opBytes)
		if err != nil {
			return err
		}
		currOffset += currLength
		currLength += uint64(writtenBytes)

		changeLocations = append(changeLocations, &jamsyncpb.ChangeLocation{
			Offset: currOffset,
			Length: currLength,
		})
		fmt.Println(in.PathHash, currOffset, currLength, opBytes)
	}

	return nil
}

func (s JamsyncStore) CommitChange(ctx context.Context, in *jamsyncpb.CommitChangeRequest) (*jamsyncpb.CommitChangeResponse, error) {
	return nil, nil
}

// func getLocalProjectDirectory(projectName string) string {
// return fmt.Sprintf(localChangeDirectory + "/" + projectName)
// }

// func (s LocalChangeStore) WriteFile(projectName string, path string, data []byte) (int64, int, error) {
// ops := generateRsyncOpsForNewFile(data)

// opsPb := make([]*jamsyncpb.Operation, 0)
// for _, op := range ops {
// opPb := rsync.RsyncOperationToPb(&op)
// opsPb = append(opsPb, &opPb)
// }
// return s.WriteChangeData(projectName, path, &jamsyncpb.ChangeData{
// Ops: opsPb,
// })
// }

// func generateRsyncOpsForNewFile(data []byte) []rsync.Operation {
// rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}

// sourceBuffer := bytes.NewReader(data)

// opsOut := make([]rsync.Operation, 0)
// var blockCt, blockRangeCt, dataCt, bytes int
// err := rsDelta.CreateDelta(sourceBuffer, []rsync.BlockHash{}, func(op rsync.Operation) error {
// switch op.Type {
// case rsync.OpBlockRange:
// blockRangeCt++
// case rsync.OpBlock:
// blockCt++
// case rsync.OpData:
// // Copy data buffer so it may be reused in internal buffer.
// b := make([]byte, len(op.Data))
// copy(b, op.Data)
// op.Data = b
// dataCt++
// bytes += len(op.Data)
// }
// opsOut = append(opsOut, op)
// return nil
// })
// if err != nil {
// log.Fatalf("Failed to create delta: %s", err)
// }

// return opsOut
// }

// func dataFilePath(projectDir string, path string) string {
// dir := getLocalProjectDirectory(projectDir)
// return fmt.Sprintf("%s/%s.jb", dir, base64.StdEncoding.EncodeToString([]byte(path)))
// }

// func (s LocalChangeStore) WriteChangeData(projectName string, path string, changeData *jamsyncpb.ChangeData) (int64, int, error) {
// dir := getLocalProjectDirectory(projectName)
// err := os.MkdirAll(dir, os.ModePerm)
// if err != nil {
// return 0, 0, err
// }

// f, err := os.OpenFile(dataFilePath(dir, path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// if err != nil {
// return 0, 0, err
// }

// info, err := f.Stat()
// if err != nil {
// return 0, 0, err
// }

// bytes, err := proto.Marshal(changeData)
// if err != nil {
// return 0, 0, err
// }

// n, err := f.Write(bytes)
// if err != nil {
// log.Fatal(err)
// return 0, 0, err
// }

// return info.Size(), n, nil
// }

// type ChangeLocation struct {
// Offset uint64
// Length uint64
// }

// func (s LocalChangeStore) ReadChangeDatas(projectName string, path string, changeLocations []ChangeLocation) ([]*jamsyncpb.ChangeData, error) {
// f, err := os.Open(dataFilePath(getLocalProjectDirectory(projectName), path))
// if err != nil {
// return nil, err
// }

// changeDatas := make([]*jamsyncpb.ChangeData, 0)
// for _, changeLocation := range changeLocations {
// changeFile := make([]byte, changeLocation.Length)
// n, err := f.ReadAt(changeFile, int64(changeLocation.Offset))
// if err != nil {
// return nil, err
// }
// if n != int(changeLocation.Length) {
// return nil, errors.New("read length does not equal expected")
// }

// change := &jamsyncpb.ChangeData{}
// err = proto.Unmarshal(changeFile, change)
// if err != nil {
// return nil, err
// }
// changeDatas = append(changeDatas, change)
// }
// return changeDatas, nil
// }

// func RegenFile(s ChangeStore, projectName string, path string, changeLocations []ChangeLocation) (*bytes.Reader, error) {
// changeDatas, err := s.ReadChangeDatas(projectName, path, changeLocations)
// if err != nil {
// return nil, err
// }

// changeOps := make([][]rsync.Operation, 0, len(changeDatas))
// for _, change := range changeDatas {
// opsOut := make([]rsync.Operation, 0, len(change.GetOps()))

// for _, op := range change.GetOps() {
// opsOut = append(opsOut, rsync.PbOperationToRsync(op))
// }
// changeOps = append(changeOps, opsOut)
// }

// rs := rsync.RSync{UniqueHasher: xxhash.New()}
// targetBuffer := bytes.NewReader([]byte{})
// result := new(bytes.Buffer)
// for _, ops := range changeOps {
// err := rs.ApplyDeltaBatch(result, targetBuffer, ops)
// if err != nil {
// return nil, err
// }
// resBytes := result.Bytes()
// targetBuffer = bytes.NewReader(resBytes)
// result.Reset()
// }

// return targetBuffer, nil
// }
