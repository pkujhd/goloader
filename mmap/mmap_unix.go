//go:build (darwin && !arm64) || (linux && !arm64) || dragonfly || freebsd || openbsd || solaris || netbsd
// +build darwin,!arm64 linux,!arm64 dragonfly freebsd openbsd solaris netbsd

package mmap

import (
	"os"
	"syscall"
)

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
}

func Mmap(size int) ([]byte, error) {
	return AcquireMapping(size, mmapCode)
}

func MmapData(size int) ([]byte, error) {
	return AcquireMapping(size, mmapData)
}

func mmapCode(size int, addr uintptr) ([]byte, error) {
	fixed := 0
	if addr != 0 {
		fixed = syscall.MAP_FIXED
	}
	data, err := mapper.Mmap(
		addr,
		0,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_PRIVATE|syscall.MAP_ANON|fixed)
	if err != nil {
		err = os.NewSyscallError("syscall.Mmap", err)
	}
	return data, err
}

func mmapData(size int, addr uintptr) ([]byte, error) {
	fixed := 0
	if addr != 0 {
		fixed = syscall.MAP_FIXED
	}
	data, err := mapper.Mmap(
		addr,
		0,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_PRIVATE|syscall.MAP_ANON|fixed)
	if err != nil {
		err = os.NewSyscallError("syscall.Mmap", err)
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
