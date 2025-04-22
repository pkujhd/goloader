//go:build windows
// +build windows

package libdl

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

func Open(dllName string) (uintptr, error) {
	dll, err := syscall.LoadDLL(dllName)
	if err != nil {
		return 0, fmt.Errorf("could not open ", dllName)
	}
	return uintptr(unsafe.Pointer(dll)), nil
}

func LookupSymbol(h uintptr, symName string) (uintptr, error) {
	index := strings.Index(symName, "%")
	symName = symName[:index]
	dll := (*syscall.DLL)(unsafe.Pointer(h))
	proc, err := dll.FindProc(symName)
	if err != nil {
		return 0, fmt.Errorf("failed to lookup symbol %s: %w", symName, err)
	}
	return proc.Addr(), nil
}
