//go:build linux || freebsd
// +build linux freebsd

package mmap

import (
	"syscall"
)

func mmap(addr uintptr, length uintptr, prot int, flags int, fd int, offset int64) (xaddr uintptr, err error) {
	r0, _, e1 := syscall.Syscall6(syscall.SYS_MMAP, addr, length, uintptr(prot), uintptr(flags), uintptr(fd), uintptr(offset))
	xaddr = r0
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

func munmap(addr uintptr, length uintptr) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_MUNMAP, addr, length, 0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}
