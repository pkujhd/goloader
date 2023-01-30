//go:build linux && arm64
// +build linux,arm64

package mmap

import (
	"os"
	"syscall"
)

func clearInstructionCacheLine(addr uintptr)

func clearDataCacheLine(addr uintptr)

func read_CTR_ELO_Register() (regval uint32)

func dataSyncBarrierInnerShareableDomain()

func instructionSyncBarrier()

const CTR_IDC_BIT = 28
const CTR_DIC_BIT = 29

var cacheInfo uint32 // Copy of cache type register contents

var iCacheLineSize uint32
var dCacheLineSize uint32

func alignAddress(addr, alignment uintptr) uintptr {
	return addr &^ (alignment - 1)
}

// Inspired by gcc's __aarch64_sync_cache_range()
func aarch64SyncInstructionCacheRange(start uintptr, end uintptr) {
	if cacheInfo == 0 {
		cacheInfo = read_CTR_ELO_Register()
	}
	iCacheLineSize = 4 << (cacheInfo & 0xF)
	dCacheLineSize = 4 << ((cacheInfo >> 16) & 0xF)

	if (cacheInfo & (CTR_IDC_BIT << 0x1)) == 0 {
		start = alignAddress(start, uintptr(dCacheLineSize))
		for addr := start; addr < end; addr += uintptr(dCacheLineSize) {
			clearDataCacheLine(addr)
		}
	}
	dataSyncBarrierInnerShareableDomain()

	if (cacheInfo & (CTR_DIC_BIT << 0x1)) == 0 {
		start = alignAddress(start, uintptr(iCacheLineSize))
		for addr := start; addr < end; addr += uintptr(iCacheLineSize) {
			clearInstructionCacheLine(addr)
		}
	}
	dataSyncBarrierInnerShareableDomain()
	instructionSyncBarrier()
}

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
	aarch64SyncInstructionCacheRange(ptr, ptr+uintptr(len))
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
