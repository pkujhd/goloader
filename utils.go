package goloader

import (
	"cmd/objfile/sys"
	"encoding/binary"
	"fmt"
	"strconv"
	"unsafe"

	"github.com/pkujhd/goloader/mmap"
	"github.com/pkujhd/goloader/obj"
)

func isOverflowInt32(offset int) bool {
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		return true
	}
	return false
}

func isOverflowInt24(offset int) bool {
	if offset > 0x7FFFFF || offset < -0x800000 {
		return true
	}
	return false
}

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

func bytearrayAlignNops(arch *sys.Arch, b *[]byte, align int) {
	length := len(*b)
	if length%align != 0 {
		*b = append(*b, createArchNops(arch, align-length%align)...)
	}
}

func createArchNops(arch *sys.Arch, size int) []byte {
	switch arch.Name {
	case sys.ArchARM.Name, sys.ArchARM64.Name:
		return createArm64Nops(size)
	case sys.Arch386.Name, sys.ArchAMD64.Name:
		return createX86Amd64Nops(size)
	default:
		panic(fmt.Errorf("not support arch:%s", arch.Name))
	}
}

func createX86Amd64Nops(size int) []byte {
	nops := make([]byte, size)
	for i := 0; i < size; i++ {
		copy(nops[i:], x86amd64NOPcode)
	}
	return nops
}

func createArm64Nops(size int) []byte {
	if size%sys.ArchARM.MinLC != 0 {
		panic(fmt.Sprintf("can't make nop instruction if padding is not multiple of %d, got %d", sys.ArchARM.MinLC, size))
	}
	nops := make([]byte, size)
	for i := 0; i < size; i += sys.ArchARM.MinLC {
		copy(nops[i:], arm64Nopcode)
	}
	return nops
}

func putAddressAddOffset(byteOrder binary.ByteOrder, b []byte, offset *int, addr uint64) {
	if PtrSize == Uint32Size {
		byteOrder.PutUint32(b[*offset:], uint32(addr))
	} else {
		byteOrder.PutUint64(b[*offset:], uint64(addr))
	}
	*offset = *offset + PtrSize
}

func putAddress(byteOrder binary.ByteOrder, b []byte, addr uint64) {
	if PtrSize == Uint32Size {
		byteOrder.PutUint32(b, uint32(addr))
	} else {
		byteOrder.PutUint64(b, uint64(addr))
	}
}

// sign extend a 24-bit integer
func signext24(x int64) int32 {
	return (int32(x) << 8) >> 8
}

func copy2Slice(dst []byte, src uintptr, size int) {
	s := sliceHeader{
		Data: src,
		Len:  size,
		Cap:  size,
	}
	copy(dst, *(*[]byte)(unsafe.Pointer(&s)))
}

func append2Slice(dst *[]byte, src uintptr, size int) {
	s := sliceHeader{
		Data: src,
		Len:  size,
		Cap:  size,
	}
	*dst = append(*dst, *(*[]byte)(unsafe.Pointer(&s))...)
}

// see runtime.internal.atomic.Loadp
//
//go:nosplit
//go:noinline
func loadp(ptr unsafe.Pointer) unsafe.Pointer {
	return *(*unsafe.Pointer)(ptr)
}

//go:inline
func grow(bytes *[]byte, size int) {
	obj.Grow(bytes, size)
}

func getArch(archName string) *sys.Arch {
	arch := &sys.Arch{}
	for index := range sys.Archs {
		if archName == sys.Archs[index].Name {
			arch = sys.Archs[index]
		}
	}
	return arch
}

func Mmap(size int) ([]byte, error) {
	return mmap.Mmap(size)
}

func MmapData(size int) ([]byte, error) {
	return mmap.MmapData(size)
}

func Munmap(b []byte) (err error) {
	return mmap.Munmap(b)
}

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
	mmap.MakeThreadJITCodeExecutable(ptr, len)
}

// see $GOROOT/src/cmd/internal/loader/loader.go:preprocess
func ispreprocesssymbol(name string) bool {
	if len(name) > 5 {
		switch name[:5] {
		case "$f32.", "$f64.", "$i64.":
			return true
		default:
		}
	}
	return false
}

func preprocesssymbol(byteOrder binary.ByteOrder, name string, bytes []byte) error {
	val, err := strconv.ParseUint(name[5:], 16, 64)
	if err != nil {
		return fmt.Errorf("failed to parse $-symbol %s: %v", name, err)
	}
	switch name[:5] {
	case "$f32.":
		if uint64(uint32(val)) != val {
			return fmt.Errorf("$-symbol %s too large: %d", name, val)
		}
		byteOrder.PutUint32(bytes, uint32(val))
		bytes = bytes[:4]
	case "$f64.", "$i64.":
		byteOrder.PutUint64(bytes, val)
	default:
		return fmt.Errorf("unrecognized $-symbol: %s", name)
	}
	return nil
}
