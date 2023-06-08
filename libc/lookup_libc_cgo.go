//go:build cgo && !windows
// +build cgo,!windows

package libc

import (
	"github.com/eh-steve/goloader/libc/libc_cgo"
)

func LookupDynamicSymbol(symName string) (uintptr, error) {
	return libc_cgo.LookupDynamicSymbol(symName)
}
