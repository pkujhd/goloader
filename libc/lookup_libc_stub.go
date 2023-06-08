//go:build (!cgo && !darwin) || windows
// +build !cgo,!darwin windows

package libc

import "fmt"

func LookupDynamicSymbol(symName string) (uintptr, error) {
	return 0, fmt.Errorf("failed to lookup symbol %s (stub unable to use libdl)", symName)
}
