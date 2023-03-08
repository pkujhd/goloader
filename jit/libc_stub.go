//go:build !cgo || windows
// +build !cgo windows

package jit

import "fmt"

func LookupDynamicSymbol(symName string) (uintptr, error) {
	return 0, fmt.Errorf("failed to lookup symbol %s", symName)
}
