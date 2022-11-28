//go:build darwin && arm64
// +build darwin,arm64

package mmap

import (
	"github.com/pkujhd/goloader/mmap/darwin_arm64"
	"os"
	"syscall"
)

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
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
		0,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_PRIVATE|syscall.MAP_ANON|syscall.MAP_JIT)
	darwin_arm64.WriteProtect()
	return data, err
}

func mmapData(size int, addr uintptr) ([]byte, error) {
	// darwin arm64 won't accept MAP_FIXED, but seems to take addr as a hint...
	data, err := mapper.Mmap(
		addr,
		0,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_PRIVATE|syscall.MAP_ANON)
	return data, err
}

func Munmap(b []byte) (err error) {
	err = mapper.Munmap(b)
	if err != nil {
		err = os.NewSyscallError("syscall.Munmap", err)
	}
	return
}
