//go:build go1.23 && !go1.28
// +build go1.23,!go1.28

package link

import (
	"unsafe"
)

var doInit1 func(t unsafe.Pointer) = nil

func addInitLinkName(symPtr map[string]uintptr) {
	*(*uintptr)(unsafe.Pointer(&doInit1)) = getFuncPointer(symPtr, "runtime.doInit1")
}
