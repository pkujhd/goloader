package mmap

import (
	"fmt"
	"github.com/pkujhd/goloader/mmap/mapping"
	"sort"
	"syscall"
	"unsafe"
)

func init() {
	pageSize = uintptr(GetAllocationGranularity())
}

const (
	MEM_COMMIT  = 0x1000
	MEM_FREE    = 0x10000
	MEM_RESERVE = 0x2000
)

const (
	MEM_IMAGE   = 0x1000000
	MEM_MAPPED  = 0x40000
	MEM_PRIVATE = 0x20000
)

func getCurrentProcMaps() ([]mapping.Mapping, error) {
	var mappings []mapping.Mapping

	pHandle, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, fmt.Errorf("failed to GetCurrentProcess: %w", err)
	}
	info := new(MemoryBasicInformation)

	addr := uintptr(0)
	infoSize := unsafe.Sizeof(*info)
	for {
		infoSize, err = VirtualQueryEx(pHandle, addr, info)
		if err != nil {
			return nil, fmt.Errorf("failed to VirtualQueryEx: %w", err)
		}
		if infoSize != unsafe.Sizeof(*info) {
			break
		}

		if info.State != MEM_FREE {
			mappings = append(mappings, mapping.Mapping{
				StartAddr:   addr,
				EndAddr:     addr + info.RegionSize,
				ReadPerm:    info.AllocationProtect&syscall.PAGE_EXECUTE_READWRITE != 0 || info.AllocationProtect&syscall.PAGE_EXECUTE_READ != 0 || info.AllocationProtect&syscall.PAGE_READWRITE != 0 || info.AllocationProtect&syscall.PAGE_READONLY != 0,
				WritePerm:   info.AllocationProtect&syscall.PAGE_EXECUTE_READWRITE != 0 || info.AllocationProtect&syscall.PAGE_EXECUTE_WRITECOPY != 0 || info.AllocationProtect&syscall.PAGE_READWRITE != 0 || info.AllocationProtect&syscall.PAGE_WRITECOPY != 0,
				ExecutePerm: info.AllocationProtect&syscall.PAGE_EXECUTE_READWRITE != 0 || info.AllocationProtect&syscall.PAGE_EXECUTE_READ != 0 || info.AllocationProtect&syscall.PAGE_EXECUTE_WRITECOPY != 0,
				PrivatePerm: info.State&MEM_PRIVATE != 0,
			})
		}
		addr += info.RegionSize
	}
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].StartAddr < mappings[j].StartAddr
	})

	err = syscall.CloseHandle(pHandle)
	if err != nil {
		return nil, fmt.Errorf("failed to CloseHandle: %w", err)
	}
	return mappings, nil
}
