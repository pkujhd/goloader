//go:build darwin && arm64
// +build darwin,arm64

package mmap

/*
#cgo darwin LDFLAGS: -lpthread

#include <pthread.h>
#include <libkern/OSCacheControl.h>

void jit_write_protect(int enable) {
	pthread_jit_write_protect_np(enable);
}
void cache_invalidate(void* start, size_t len) {
	sys_icache_invalidate(start, len);
}
*/
import "C"

import (
	"os"
	"syscall"
	"unsafe"
)

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
	C.jit_write_protect(C.int(1))
	C.cache_invalidate(unsafe.Pointer(ptr), C.size_t(len))
}

func Mmap(size int) ([]byte, error) {
	data, err := syscall.Mmap(
		0,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_PRIVATE|syscall.MAP_ANON|syscall.MAP_JIT)
	if err != nil {
		err = os.NewSyscallError("syscall.Mmap", err)
	}
	C.jit_write_protect(C.int(0))
	return data, err
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

func Munmap(b []byte) (err error) {
	err = syscall.Munmap(b)
	if err != nil {
		err = os.NewSyscallError("syscall.Munmap", err)
	}
	return
}
