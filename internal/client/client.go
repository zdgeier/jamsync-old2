package client

import (
	"bytes"
	"context"
	"io"
	"log"

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
		log.Printf("Range Ops:%5d, Block Ops:%5d, Data Ops: %5d, Data Len: %5dB", blockRangeCt, blockCt, dataCt, bytes)
		if err != nil {
			panic(err)
		}
	}()

	writeStream, err := client.WriteOperationStream(ctx)
	if err != nil {
		return err
	}
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
	}
	_, err = writeStream.CloseAndRecv()
	return err
}

func GetFileListDiff(ctx context.Context, client pb.JamsyncAPIClient, fileMetadata *pb.FileMetadata, projectId uint64, changeId uint64) (*pb.FileMetadataDiff, error) {
	fileMetadataData, err := proto.Marshal(fileMetadata)
	if err != nil {
		return nil, err
	}
	fileMetadataReader := bytes.NewReader(fileMetadataData)

	rs := rsync.RSync{UniqueHasher: xxhash.New()}
	blockHashes := make([]*pb.BlockHash, 0)
	err = rs.CreateSignature(fileMetadataReader, func(bl rsync.BlockHash) error {
		blockHashes = append(blockHashes, &pb.BlockHash{
			Index:      bl.Index,
			StrongHash: bl.StrongHash,
			WeakHash:   bl.WeakHash,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	readFileClient, err := client.ReadFile(ctx, &pb.ReadFileRequest{
		ProjectId:   projectId,
		ChangeId:    changeId,
		PathHash:    pathToHash(".jamsyncfilelist"),
		ModTime:     timestamppb.Now(),
		BlockHashes: blockHashes,
	})
	if err != nil {
		return nil, err
	}

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
		}
		close(ops)
	}()

	result := new(bytes.Buffer)
	fileMetadataReader.Reset(fileMetadataData)
	err = rs.ApplyDelta(result, fileMetadataReader, ops)
	if err != nil {
		return nil, err
	}

	remoteFileMetadata := &pb.FileMetadata{}
	err = proto.Unmarshal(result.Bytes(), remoteFileMetadata)
	if err != nil {
		return nil, err
	}

	fileMetadataDiff := make(map[string]*pb.FileMetadataDiff_FileDiff, len(fileMetadata.GetFiles()))
	for filePath, file := range fileMetadata.GetFiles() {
		fileMetadataDiff[filePath] = &pb.FileMetadataDiff_FileDiff{
			Type: pb.FileMetadataDiff_NoOp,
			File: file,
		}
	}

	for remoteFilePath, remoteFile := range remoteFileMetadata.GetFiles() {
		diffType := pb.FileMetadataDiff_NoOp
		diffFile, found := fileMetadataDiff[remoteFilePath]
		if found && remoteFile.Hash != fileMetadata.GetFiles()[remoteFilePath].GetHash() {
			diffType = pb.FileMetadataDiff_Update
		} else {
			diffType = pb.FileMetadataDiff_Create
		}

		fileMetadataDiff[remoteFilePath] = &pb.FileMetadataDiff_FileDiff{
			Type: diffType,
			File: diffFile.File,
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
		}
		close(ops)
	}()

	localReader.Seek(0, 0)
	return rs.ApplyDelta(localWriter, localReader, ops)
}

func pathToHash(path string) uint64 {
	h := xxhash.New()
	h.Write([]byte(path))
	return h.Sum64()
}
