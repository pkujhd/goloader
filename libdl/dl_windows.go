//go:build windows
// +build windows

package libdl

import (
	"fmt"
	"syscall"
)

var (
	modkernel32 *syscall.DLL
)

func Open(symName string) (uintptr, error) {
	if modkernel32 == nil {
		var err error
		modkernel32, err = syscall.LoadDLL("kernel32.dll")
		if err != nil {
			return 0, fmt.Errorf("could not open kernel32.dll")
		}
	}
	return 0, nil
}

func LookupSymbol(h uintptr, symName string) (uintptr, error) {
	proc, err := modkernel32.FindProc(symName)
	if err != nil {
		return 0, fmt.Errorf("failed to lookup symbol %s: %w", symName, err)
	}
	return proc.Addr(), nil
}
