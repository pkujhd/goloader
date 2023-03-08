//go:build darwin && arm64
// +build darwin,arm64

package mmap

import (
	"github.com/eh-steve/goloader/mmap/darwin_arm64"
	"github.com/eh-steve/goloader/mprotect"

	"fmt"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
	var pages []byte
	pageSlice := (*reflect.SliceHeader)(unsafe.Pointer(&pages))
	pageSlice.Data = ptr
	pageSlice.Len = len
	pageSlice.Cap = len
	err := mprotect.MprotectMakeExecutable(pages)
	if err != nil {
		panic(err)
	}
	darwin_arm64.MakeThreadJITCodeExecutable(ptr, len)
}

func Mmap(size int) ([]byte, error) {
	return AcquireMapping(size, mmapCode)
}

func MmapData(size int) ([]byte, error) {
	return AcquireMapping(size, mmapData)
}

func mmapCode(size int, addr uintptr) ([]byte, error) {
	// darwin arm64 won't accept MAP_FIXED, but seems to take addr as a hint...
	data, err := mapper.Mmap(
		addr,
		-1,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE, // this is not yet executable, we will mprotect it after we're finished writing to it
		syscall.MAP_PRIVATE|syscall.MAP_ANON|syscall.MAP_JIT)
	if err != nil {
		return nil, err
	}
	if uintptr(unsafe.Pointer(&data[0]))-addr > 1<<24 {
		defer mapper.Munmap(data)
		return nil, fmt.Errorf("failed to acquire code mapping within 24 bit address of 0x%x, got %p", addr, &data[0])
	}
	darwin_arm64.WriteProtectDisable()
	return data, err
}

func mmapData(size int, addr uintptr) ([]byte, error) {
	// darwin arm64 won't accept MAP_FIXED, but seems to take addr as a hint...
	data, err := mapper.Mmap(
		addr,
		-1,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		return nil, err
	}
	if uintptr(unsafe.Pointer(&data[0]))-addr > 1<<24 {
		defer mapper.Munmap(data)
		return nil, fmt.Errorf("failed to acquire data mapping within 24 bit address of 0x%x, got %p", addr, &data[0])
	}
	return data, err
}

func Munmap(b []byte) (err error) {
	err = mapper.Munmap(b)
	if err != nil {
		err = os.NewSyscallError("syscall.Munmap", err)
	}
	return
}
