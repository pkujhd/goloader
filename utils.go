package goloader

import (
	"bytes"
	"unsafe"
)

//go:linkname add runtime.add
func add(p unsafe.Pointer, x uintptr) unsafe.Pointer

func assert(err error) {
	if err != nil {
		panic(err)
	}
}

func PutUint24(b []byte, v uint32) {
	_ = b[2] // early bounds check to guarantee safety of writes below
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
}

func sprintf(buf *bytes.Buffer, str ...string) {
	for _, s := range str {
		buf.WriteString(s)
	}
}

func copy2Slice(dst []byte, src unsafe.Pointer, size int) {
	var s = sliceHeader{
		Data: (uintptr)(src),
		Len:  size,
		Cap:  size,
	}
	copy(dst, *(*[]byte)(unsafe.Pointer(&s)))
}
