//go:build windows
// +build windows

package mmap

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32                         = syscall.NewLazyDLL("kernel32.dll")
	modntdll                            = syscall.NewLazyDLL("ntdll.dll")
	procMapViewOfFileEx                 = modkernel32.NewProc("MapViewOfFileEx")
	procCreateToolhelp32Snapshot        = modkernel32.NewProc("CreateToolhelp32Snapshot")
	procModule32First                   = modkernel32.NewProc("Module32First")
	procModule32Next                    = modkernel32.NewProc("Module32Next")
	procRtlCreateQueryDebugBuffer       = modntdll.NewProc("RtlCreateQueryDebugBuffer")
	procRtlQueryProcessDebugInformation = modntdll.NewProc("RtlQueryProcessDebugInformation")
	procRtlDestroyQueryDebugBuffer      = modntdll.NewProc("RtlDestroyQueryDebugBuffer")
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

func CreateToolhelp32Snapshot(flags uint32, pid uint32) (handle syscall.Handle, err error) {
	r0, _, e1 := syscall.SyscallN(procCreateToolhelp32Snapshot.Addr(), uintptr(flags), uintptr(pid))
	handle = syscall.Handle(r0)
	if handle == syscall.InvalidHandle {
		err = errnoErr(e1)
	}
	return
}

const MAX_MODULE_NAME32 = 255
const MAX_PATH = 260

// ModuleEntry32 defined in tlhelp32.h - the struct padding in Go happens to match C++
type ModuleEntry32 struct {
	size          uint64
	th32ModuleID  uint32
	th32ProcessID uint32
	GlblcntUsage  uint32
	ProccntUsage  uint32
	modBaseAddr   uint64
	modBaseSize   int32
	hModule       uint64
	szModule      [MAX_MODULE_NAME32 + 1]byte
	szExePath     [MAX_PATH]byte
}

func Module32First(handle syscall.Handle, m *ModuleEntry32) (ok bool, err error) {
	m.size = uint64(unsafe.Sizeof(*m))
	r0, _, e1 := syscall.SyscallN(procModule32First.Addr(), uintptr(handle), uintptr(unsafe.Pointer(m)))
	if r0 == 0 {
		err = errnoErr(e1)
	} else {
		ok = true
	}
	return
}

func Module32Next(handle syscall.Handle, m *ModuleEntry32) (ok bool, err error) {
	m.size = uint64(unsafe.Sizeof(*m))
	r0, _, e1 := syscall.SyscallN(procModule32Next.Addr(), uintptr(handle), uintptr(unsafe.Pointer(m)))
	if r0 == 0 {
		err = errnoErr(e1)
	} else {
		ok = true
	}
	return
}

type DebugBuffer struct {
	SectionHandle        uintptr
	SectionBase          uintptr
	RemoteSectionBase    uintptr
	SectionBaseDelta     uintptr
	EventPairHandle      uintptr
	Unknown              [2]uintptr
	RemoteThreadHandle   uintptr
	InfoClassMask        uint32
	SizeOfInfo           uintptr
	AllocatedSize        uintptr
	SectionSize          uintptr
	ModuleInformation    uintptr
	BackTraceInformation uintptr
	HeapInformation      *uint64
	LockInformation      uintptr
	Reserved             [8]uintptr
}

type DebugHeapInfo struct {
	Base        uintptr
	Flags       uint32
	Granularity uint16
	Unknown     uint16
	Allocated   uintptr
	Committed   uintptr
	TagCount    uint32
	BlockCount  uint32
	Reserved    [7]uint32
	Tags        uintptr
	Blocks      uintptr
}

func RtlCreateQueryDebugBuffer() (buf *DebugBuffer, err error) {
	r0, _, e1 := syscall.SyscallN(procRtlCreateQueryDebugBuffer.Addr(), 0, 0)
	if r0 == 0 {
		err = errnoErr(e1)
	} else {
		buf = (*DebugBuffer)(unsafe.Pointer(r0))
	}
	return
}

// RtlQueryProcessDebugInformation.DebugInfoClassMask constants
const PDI_MODULES = 0x01
const PDI_BACKTRACE = 0x02
const PDI_HEAPS = 0x04
const PDI_HEAP_TAGS = 0x08
const PDI_HEAP_BLOCKS = 0x10
const PDI_LOCKS = 0x20

func RtlQueryProcessDebugInformation(buf *DebugBuffer) (err error) {
	r0, _, e1 := syscall.SyscallN(procRtlQueryProcessDebugInformation.Addr(), uintptr(os.Getpid()), PDI_HEAPS|PDI_HEAP_BLOCKS, uintptr(unsafe.Pointer(buf)))
	if r0 < 0 {
		err = errnoErr(e1)
	}
	return
}

func RtlDestroyQueryDebugBuffer(buf *DebugBuffer) (err error) {
	r0, _, e1 := syscall.SyscallN(procRtlDestroyQueryDebugBuffer.Addr(), uintptr(unsafe.Pointer(buf)))
	if r0 < 0 {
		err = errnoErr(e1)
	}
	return
}
