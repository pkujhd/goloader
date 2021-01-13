package goloader

import "io"

type readAtSeeker struct {
	io.ReadSeeker
}

func (r *readAtSeeker) BytesAt(offset, size int64) (bytes []byte, err error) {
	bytes = make([]byte, size)
	_, err = r.Seek(offset, io.SeekStart)
	if err == nil {
		_, err = r.Read(bytes)
	}
	return
}
