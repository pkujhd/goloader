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
		syscall.MAP_PRIVATE|syscall.MAP_ANON|syscall.MAP_JIT|fixed)
	if err != nil {
		err = os.NewSyscallError("syscall.Mmap", err)
	}
	C.jit_write_protect(C.int(0))
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
