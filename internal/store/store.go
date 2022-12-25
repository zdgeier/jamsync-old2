package store

import (
	"fmt"
)

type Store interface {
	Read(projectId uint64, changeId uint64, pathHash uint64, offset uint64, length uint64) (data []byte, err error)
	Write(projectId uint64, changeId uint64, pathHash uint64, data []byte) (offset uint64, length uint64, err error)
}

//
// type LocalStore struct {
// 	directory string
// }
//
// func (s LocalStore) changeDirectory(projectId uint64) string {
// 	return fmt.Sprintf("%s/%d", s.directory, projectId)
// }
// func (s LocalStore) filePath(projectId uint64, changeId uint64, pathHash uint64) string {
// 	return fmt.Sprintf("%s/%d.jb", s.changeDirectory(projectId), pathHash)
// }
// func (s LocalStore) Read(projectId uint64, changeId uint64, pathHash uint64, offset uint64, length uint64) (data []byte, err error) {
// 	currFile, err := os.OpenFile(s.filePath(projectId, changeId, pathHash), os.O_RDONLY, 0644)
// 	if err != nil {
// 		return nil, err
// 	}
// 	b := make([]byte, length)
// 	_, err = currFile.ReadAt(b, int64(offset))
// 	if err != nil {
// 		return nil, err
// 	}
// 	return b, nil
// }
// func (s LocalStore) Write(projectId uint64, changeId uint64, pathHash uint64, data []byte) (offset uint64, length uint64, err error) {
// 	currFile, err := os.OpenFile(s.filePath(projectId, changeId, pathHash), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// 	if err != nil {
// 		return 0, 0, err
// 	}
// 	info, err := currFile.Stat()
// 	if err != nil {
// 		return 0, 0, err
// 	}
// 	writtenBytes, err := currFile.Write(data)
// 	if err != nil {
// 		return 0, 0, err
// 	}
// 	return uint64(info.Size()), uint64(writtenBytes), nil
// }

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

	curr := s.files[s.filePath(projectId, changeId, pathHash)]
	curr = append(curr[:], data[:]...)
	s.files[s.filePath(projectId, changeId, pathHash)] = curr

	return offset, length, nil
}
