//go:build go1.23 && !go1.28
// +build go1.23,!go1.28

package obj

import (
	"fmt"
	"unsafe"
)

var (
	ptrSlice [16]uintptr
	ptrIndex = 0
)

//go:inline
func getFuncPointer(symPtr map[string]uintptr, funcName string) uintptr {
	ptrIndex++
	ptrSlice[ptrIndex] = symPtr[funcName]
	if ptrSlice[ptrIndex] != 0 {
		return (uintptr)(unsafe.Pointer(&ptrSlice[ptrIndex]))
	}
	panic(fmt.Errorf("not found function:%s in runtime", funcName))
}

var (
	_name func(n Name) string = nil
)

func AddLinkName(symPtr map[string]uintptr) {
	AddInstLinkName(symPtr)

	*(*uintptr)(unsafe.Pointer(&_name)) = getFuncPointer(symPtr, "internal/abi.Name.Name")
}
