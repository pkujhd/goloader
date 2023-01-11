//go:build windows
// +build windows

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
	return VirtualProtect(uintptr(unsafe.Pointer(&page[0])), uintptr(len(page)), syscall.PAGE_READWRITE)
}

func MprotectMakeExecutable(page []byte) error {
	return VirtualProtect(uintptr(unsafe.Pointer(&page[0])), uintptr(len(page)), syscall.PAGE_EXECUTE_READ)
}

func MprotectMakeReadOnly(page []byte) error {
	return VirtualProtect(uintptr(unsafe.Pointer(&page[0])), uintptr(len(page)), syscall.PAGE_READONLY)
}
