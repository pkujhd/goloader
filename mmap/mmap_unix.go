//go:build (darwin && amd64) || dragonfly || freebsd || (linux && !amd64) || openbsd || solaris || netbsd
// +build darwin,amd64 dragonfly freebsd linux,!amd64 openbsd solaris netbsd

package mmap

import (
	"os"
	"syscall"
)

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
}

func MmapData(size int) ([]byte, error) {
	data, err := syscall.Mmap(
		0,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		err = os.NewSyscallError("syscall.Mmap", err)
	}
	return data, err
}

func Mmap(size int) ([]byte, error) {
	data, err := syscall.Mmap(
		0,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		err = os.NewSyscallError("syscall.Mmap", err)
	}
	return data, err
}

func Munmap(b []byte) (err error) {
	err = syscall.Munmap(b)
	if err != nil {
		err = os.NewSyscallError("syscall.Munmap", err)
	}
	return
}
