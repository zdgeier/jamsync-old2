package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryStore(t *testing.T) {
	type writeData struct {
		projectId uint64
		changeId  uint64
		pathHash  uint64
		data      []byte
	}
	type writeDataResult struct {
		offset uint64
		length uint64
		err    error
	}
	tests := []struct {
		writeData               writeData
		expectedWriteDataResult writeDataResult
	}{
		{
			writeData: writeData{
				projectId: 1,
				changeId:  1,
				pathHash:  123,
				data:      []byte("this is a test"),
			},
			expectedWriteDataResult: writeDataResult{
				offset: 0,
				length: 14,
			},
		},
		{
			writeData: writeData{
				projectId: 1,
				changeId:  1,
				pathHash:  123,
				data:      []byte("this is another test"),
			},
			expectedWriteDataResult: writeDataResult{
				offset: 14,
				length: 20,
			},
		},
	}

	store := NewMemoryStore()
	for _, test := range tests {
		offset, length, err := store.Write(test.writeData.projectId, test.writeData.changeId, test.writeData.pathHash, test.writeData.data)
		require.Equal(t, writeDataResult{
			offset: offset,
			length: length,
			err:    err,
		}, test.expectedWriteDataResult)

		data, err := store.Read(test.writeData.projectId, test.writeData.changeId, test.writeData.pathHash, offset, length)
		require.NoError(t, err)

		require.Equal(t, data, test.writeData.data)
	}
}