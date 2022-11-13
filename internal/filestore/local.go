package filestore

type Filestore struct {
}

func NewLocalFilestore() Filestore {
	return Filestore{}
}

func (f *Filestore) Write(p []byte) (n int, err error) {
	return 0, nil
}
