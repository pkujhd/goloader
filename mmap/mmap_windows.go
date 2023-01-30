//go:build windows
// +build windows

package mmap

import (
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
}

func Mmap(size int) ([]byte, error) {
	return AcquireMapping(size, mmapCode)
}

func MmapData(size int) ([]byte, error) {
	return AcquireMapping(size, mmapData)
}

func mmapCode(size int, targetAddr uintptr) ([]byte, error) {
	sizelo := uint32(size >> 32)
	sizehi := uint32(size) & 0xFFFFFFFF
	h, errno := syscall.CreateFileMapping(syscall.InvalidHandle, nil,
		syscall.PAGE_EXECUTE_READWRITE, sizelo, sizehi, nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}

	addr, errno := MapViewOfFileEx(h,
		syscall.FILE_MAP_READ|syscall.FILE_MAP_WRITE|syscall.FILE_MAP_EXECUTE,
		0, 0, uintptr(size), targetAddr)
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFileEx", errno)
	}

	if err := syscall.CloseHandle(h); err != nil {
		return nil, os.NewSyscallError("CloseHandle", err)
	}

	var header reflect.SliceHeader
	header.Data = addr
	header.Len = size
	header.Cap = size
	b := *(*[]byte)(unsafe.Pointer(&header))

	return b, nil
}

func mmapData(size int, targetAddr uintptr) ([]byte, error) {
	sizelo := uint32(size >> 32)
	sizehi := uint32(size) & 0xFFFFFFFF
	h, errno := syscall.CreateFileMapping(syscall.InvalidHandle, nil,
		syscall.PAGE_READWRITE, sizelo, sizehi, nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}

	addr, errno := MapViewOfFileEx(h,
		syscall.FILE_MAP_READ|syscall.FILE_MAP_WRITE,
		0, 0, uintptr(size), targetAddr)
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFileEx", errno)
	}

	if err := syscall.CloseHandle(h); err != nil {
		return nil, os.NewSyscallError("CloseHandle", err)
	}

	var header reflect.SliceHeader
	header.Data = addr
	header.Len = size
	header.Cap = size
	b := *(*[]byte)(unsafe.Pointer(&header))

	return b, nil
}

func Munmap(b []byte) error {
	addr := (uintptr)(unsafe.Pointer(&b[0]))
	if err := syscall.UnmapViewOfFile(addr); err != nil {
		return os.NewSyscallError("UnmapViewOfFile", err)
	}
	return nil
}
