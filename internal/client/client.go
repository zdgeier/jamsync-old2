package client

import (
	"bytes"
	"context"
	"io"

	"github.com/cespare/xxhash"
	"github.com/zdgeier/jamsync/gen/pb"
	"github.com/zdgeier/jamsync/internal/rsync"
	"github.com/zdgeier/jamsync/internal/server"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func UploadDiff(client pb.JamsyncAPIClient, fileMetadata *pb.FileMetadata, fileData map[string][]byte, projectId uint64, changeId uint64) error {
	ctx := context.Background()

	diff, err := GetFileListDiff(ctx, client, fileMetadata, projectId, changeId)
	if err != nil {
		return err
	}
	for filePath, fileDiff := range diff.GetDiffs() {
		if fileDiff.File.Dir || fileDiff.Type == pb.FileMetadataDiff_NoOp {
			continue
		}
		err := UploadFile(ctx, client, projectId, changeId, filePath, bytes.NewReader(fileData[filePath]))
		if err != nil {
			return err
		}
	}
	fileMetadataData, err := proto.Marshal(fileMetadata)
	if err != nil {
		return err
	}
	err = UploadFile(ctx, client, projectId, changeId, ".jamsyncfilemetadata", bytes.NewReader(fileMetadataData))
	if err != nil {
		return err
	}

	return nil
}

func UploadFile(ctx context.Context, client pb.JamsyncAPIClient, projectId uint64, changeId uint64, filePath string, sourceReader io.Reader) error {
	blockHashResp, err := client.ReadBlockHashes(ctx, &pb.ReadBlockHashesRequest{
		ProjectId: projectId,
		ChangeId:  changeId,
		PathHash:  pathToHash(filePath),
		ModTime:   timestamppb.Now(),
	})
	if err != nil {
		return err
	}

	opsOut := make(chan *rsync.Operation)
	rsDelta := &rsync.RSync{UniqueHasher: xxhash.New()}
	go func() {
		var blockCt, blockRangeCt, dataCt, bytes int
		defer close(opsOut)
		err := rsDelta.CreateDelta(sourceReader, server.PbBlockHashesToRsync(blockHashResp.GetBlockHashes()), func(op rsync.Operation) error {
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
			opsOut <- &op
			return nil
		})
		//log.Printf("%s: Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dB", filePath, blockRangeCt, blockCt, dataCt, bytes)
		if err != nil {
			panic(err)
		}
	}()

	writeStream, err := client.WriteOperationStream(ctx)
	if err != nil {
		return err
	}
	sent := 0
	for op := range opsOut {
		var opPbType pb.Operation_Type
		switch op.Type {
		case rsync.OpBlock:
			opPbType = pb.Operation_OpBlock
		case rsync.OpData:
			opPbType = pb.Operation_OpData
		case rsync.OpHash:
			opPbType = pb.Operation_OpHash
		case rsync.OpBlockRange:
			opPbType = pb.Operation_OpBlockRange
		}

		err = writeStream.Send(&pb.Operation{
			ProjectId:     projectId,
			ChangeId:      changeId,
			PathHash:      pathToHash(filePath),
			Type:          opPbType,
			BlockIndex:    op.BlockIndex,
			BlockIndexEnd: op.BlockIndexEnd,
			Data:          op.Data,
		})
		if err != nil {
			return err
		}
		sent += 1
	}
	// We have to send a tombstone if we have not generated any ops (empty file)
	if sent == 0 {
		writeStream.Send(&pb.Operation{
			ProjectId:     projectId,
			ChangeId:      changeId,
			PathHash:      pathToHash(filePath),
			Type:          pb.Operation_OpData,
			BlockIndex:    0,
			BlockIndexEnd: 0,
			Data:          []byte{},
		})
	}
	_, err = writeStream.CloseAndRecv()
	return err
}

func UploadFileList(ctx context.Context, client pb.JamsyncAPIClient, fileMetadata *pb.FileMetadata, projectName string) error {
	resp, err := client.CreateChange(ctx, &pb.CreateChangeRequest{
		ProjectName: projectName,
	})
	if err != nil {
		return err
	}
	metadataBytes, err := proto.Marshal(fileMetadata)
	if err != nil {
		return err
	}
	err = UploadFile(ctx, client, resp.GetProjectId(), resp.GetChangeId(), ".jamsyncfilelist", bytes.NewReader(metadataBytes))
	if err != nil {
		return err
	}
	_, err = client.CommitChange(ctx, &pb.CommitChangeRequest{
		ProjectId: resp.ProjectId,
		ChangeId:  resp.ChangeId,
	})
	return err
}

func GetFileListDiff(ctx context.Context, client pb.JamsyncAPIClient, fileMetadata *pb.FileMetadata, projectId uint64, changeId uint64) (*pb.FileMetadataDiff, error) {
	metadataBytes, err := proto.Marshal(fileMetadata)
	if err != nil {
		return nil, err
	}
	metadataReader := bytes.NewReader(metadataBytes)
	metadataResult := new(bytes.Buffer)
	err = DownloadFile(ctx, client, projectId, changeId, ".jamsyncfilelist", metadataReader, metadataResult)
	if err != nil {
		return nil, err
	}

	remoteFileMetadata := &pb.FileMetadata{}
	err = proto.Unmarshal(metadataResult.Bytes(), remoteFileMetadata)
	if err != nil {
		return nil, err
	}

	fileMetadataDiff := make(map[string]*pb.FileMetadataDiff_FileDiff, len(fileMetadata.GetFiles()))
	for remoteFilePath := range remoteFileMetadata.GetFiles() {
		fileMetadataDiff[remoteFilePath] = &pb.FileMetadataDiff_FileDiff{
			Type: pb.FileMetadataDiff_Delete,
		}
	}

	for filePath, file := range fileMetadata.GetFiles() {
		var diffFile *pb.File
		diffType := pb.FileMetadataDiff_Delete
		remoteFile, found := remoteFileMetadata.GetFiles()[filePath]
		if found && proto.Equal(file, remoteFile) {
			diffType = pb.FileMetadataDiff_NoOp
		} else if found {
			diffFile = file
			diffType = pb.FileMetadataDiff_Update
		} else {
			diffFile = file
			diffType = pb.FileMetadataDiff_Create
		}

		fileMetadataDiff[filePath] = &pb.FileMetadataDiff_FileDiff{
			Type: diffType,
			File: diffFile,
		}
	}

	return &pb.FileMetadataDiff{
		Diffs: fileMetadataDiff,
	}, err
}

func DownloadFile(ctx context.Context, client pb.JamsyncAPIClient, projectId uint64, changeId uint64, filePath string, localReader *bytes.Reader, localWriter io.Writer) error {
	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	blockHashes := make([]*pb.BlockHash, 0)
	err := rs.CreateSignature(localReader, func(bl rsync.BlockHash) error {
		blockHashes = append(blockHashes, &pb.BlockHash{
			Index:      bl.Index,
			StrongHash: bl.StrongHash,
			WeakHash:   bl.WeakHash,
		})
		return nil
	})
	if err != nil {
		return err
	}

	readFileClient, err := client.ReadFile(ctx, &pb.ReadFileRequest{
		ProjectId:   projectId,
		ChangeId:    changeId,
		PathHash:    pathToHash(filePath),
		ModTime:     timestamppb.Now(),
		BlockHashes: blockHashes,
	})
	if err != nil {
		return err
	}

	numOps := 0
	ops := make(chan rsync.Operation)
	go func() {
		for {
			in, err := readFileClient.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}
			ops <- server.PbOperationToRsync(in)
			numOps += 1
		}
		close(ops)
	}()

	localReader.Seek(0, 0)
	err = rs.ApplyDelta(localWriter, localReader, ops)
	if err != nil {
		return err
	}

	return err
}

func pathToHash(path string) uint64 {
	h := xxhash.New()
	h.Write([]byte(path))
	return h.Sum64()
}
