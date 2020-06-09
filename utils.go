package goloader

import (
	"encoding/binary"
	"unsafe"
)

//go:linkname add runtime.add
func add(p unsafe.Pointer, x uintptr) unsafe.Pointer

//go:linkname adduintptr runtime.add
func adduintptr(p uintptr, x int) unsafe.Pointer

func putUint24(b []byte, v uint32) {
	_ = b[2] // early bounds check to guarantee safety of writes below
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
}

func alignof(i int, align int) int {
	if i%align != 0 {
		i = i + (align - i%align)
	}
	return i
}

func bytearrayAlign(b *[]byte, align int) {
	length := len(*b)
	if length%align != 0 {
		*b = append(*b, make([]byte, align-length%align)...)
	}
}

func putAddressAddOffset(b []byte, offset *int, addr uint64) {
	if PtrSize == Uint32Size {
		binary.LittleEndian.PutUint32(b[*offset:], uint32(addr))
	} else {
		binary.LittleEndian.PutUint64(b[*offset:], uint64(addr))
	}
	*offset = *offset + PtrSize
}

func putAddress(b []byte, addr uint64) {
	if PtrSize == Uint32Size {
		binary.LittleEndian.PutUint32(b, uint32(addr))
	} else {
		binary.LittleEndian.PutUint64(b, uint64(addr))
	}
}

// sign extend a 24-bit integer
func signext24(x int64) int32 {
	return (int32(x) << 8) >> 8
}

func copy2Slice(dst []byte, src uintptr, size int) {
	var s = sliceHeader{
		Data: src,
		Len:  size,
		Cap:  size,
	}
	copy(dst, *(*[]byte)(unsafe.Pointer(&s)))
}

//go:nosplit
//go:noinline
//see runtime.internal.atomic.Loadp
func loadp(ptr unsafe.Pointer) unsafe.Pointer {
	return *(*unsafe.Pointer)(ptr)
}
