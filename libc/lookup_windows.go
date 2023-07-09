//go:build cgo && windows
// +build cgo,windows

package libc

/*
// Purely to bake in _cgo_topofstack _cgo_get_context_function etc

*/
import "C"
import (
	"fmt"
	"strings"
	"syscall"
)

var modkernel32, err = syscall.LoadDLL("kernel32.dll")

func init() {
	if err != nil {
		panic("could not open kernel32.dll")
	}
}

func LookupDynamicSymbol(symName string) (uintptr, error) {
	// Windows doesn't have a libdl, so we can't find self-loaded C symbols by passing a null string to dlopen,
	// so instead have to lookup in kernel32.dll.
	// TODO - should we also attempt to look in some others?
	if strings.HasPrefix(symName, "_cgo") {
		return 0, fmt.Errorf("could not find CGo symbol %s (should have been included in host binary symtab)", symName)
	}
	proc, err := modkernel32.FindProc(symName)
	if err != nil {
		return 0, fmt.Errorf("failed to lookup symbol %s: %w", symName, err)
	}
	return proc.Addr(), nil
}
