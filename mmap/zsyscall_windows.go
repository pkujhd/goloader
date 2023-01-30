//go:build windows
// +build windows

package mmap

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32         = syscall.NewLazyDLL("kernel32.dll")
	procMapViewOfFileEx = modkernel32.NewProc("MapViewOfFileEx")
	procVirtualQueryEx  = modkernel32.NewProc("VirtualQueryEx")
	procGetSystemInfo   = modkernel32.NewProc("GetSystemInfo")
)

const (
	errnoERROR_IO_PENDING = 997
)

var (
	errERROR_IO_PENDING error = syscall.Errno(errnoERROR_IO_PENDING)
	errERROR_EINVAL     error = syscall.EINVAL
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return errERROR_EINVAL
	case errnoERROR_IO_PENDING:
		return errERROR_IO_PENDING
	}
	return e
}

func MapViewOfFileEx(handle syscall.Handle, access uint32, offsetHigh uint32, offsetLow uint32, length uintptr, baseAddr uintptr) (addr uintptr, err error) {
	r0, _, e1 := syscall.SyscallN(procMapViewOfFileEx.Addr(), uintptr(handle), uintptr(access), uintptr(offsetHigh), uintptr(offsetLow), length, baseAddr)
	addr = r0
	if addr == 0 {
		err = errnoErr(e1)
	}
	return
}

type MemoryBasicInformation struct {
	BaseAddress       uintptr
	AllocationBase    uintptr
	AllocationProtect uint32
	PartitionId       uint16
	RegionSize        uintptr
	State             uint32
	Protect           uint32
	Type              uint32
}

func VirtualQueryEx(processHandle syscall.Handle, addr uintptr, m *MemoryBasicInformation) (infoSize uintptr, err error) {
	r0, _, e1 := syscall.SyscallN(procVirtualQueryEx.Addr(), uintptr(processHandle), addr, uintptr(unsafe.Pointer(m)), unsafe.Sizeof(*m))
	if r0 < 0 {
		err = errnoErr(e1)
	}
	return r0, err
}

type SystemInfo struct {
	OemId                     uint32
	PageSize                  uint32
	MinimumApplicationAddress uintptr
	MaximumApplicationAddress uintptr
	ActiveProcessorMask       uintptr
	NumberOfProcessors        uint32
	ProcessorType             uint32
	AllocationGranularity     uint32
	ProcessorLevel            uint16
	ProcessorRevision         uint16
}

func GetSystemInfo(info *SystemInfo) {
	syscall.SyscallN(procGetSystemInfo.Addr(), uintptr(unsafe.Pointer(info)))
}

func GetAllocationGranularity() uint32 {
	info := SystemInfo{}
	GetSystemInfo(&info)
	return info.AllocationGranularity
}
