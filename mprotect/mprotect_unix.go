//go:build darwin || dragonfly || freebsd || linux || openbsd || solaris || netbsd
// +build darwin dragonfly freebsd linux openbsd solaris netbsd

package mprotect

import (
	"syscall"
	"unsafe"
)

var pageSize = syscall.Getpagesize()

func GetPage(p uintptr) []byte {
	return (*(*[0xFFFFFF]byte)(unsafe.Pointer(p & ^uintptr(pageSize-1))))[:pageSize]
}

func RawMemoryAccess(b uintptr) []byte {
	return (*(*[0xFF]byte)(unsafe.Pointer(b)))[:]
}

func MprotectMakeWritable(page []byte) error {
	return syscall.Mprotect(page, syscall.PROT_READ|syscall.PROT_WRITE)
}

func MprotectMakeExecutable(page []byte) error {
	return syscall.Mprotect(page, syscall.PROT_READ|syscall.PROT_EXEC)
}

func MprotectMakeReadOnly(page []byte) error {
	return syscall.Mprotect(page, syscall.PROT_READ)
}
