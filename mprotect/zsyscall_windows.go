//go:build windows
// +build windows

package mprotect

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32        = syscall.NewLazyDLL("kernel32.dll")
	procVirtualProtect = modkernel32.NewProc("VirtualProtect")
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

func VirtualProtect(addr uintptr, length uintptr, flNewProtect uintptr) (err error) {
	var out int
	r1, _, e1 := syscall.SyscallN(procVirtualProtect.Addr(), addr, length, flNewProtect, uintptr(unsafe.Pointer(&out)))
	if r1 == 0 {
		err = errnoErr(e1)
	}
	return
}
