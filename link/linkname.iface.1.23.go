//go:build go1.23 && !go1.28
// +build go1.23,!go1.28

package link

import (
	"unsafe"
)

var (
	itabAdd func(m *itab) = nil

	_itabTable uintptr
	itabTable  *itabTableType = nil

	_itabLock uintptr
	itabLock  *mutex = nil
)

func addIfaceLinkName(symPtr map[string]uintptr) {
	*(*uintptr)(unsafe.Pointer(&itabAdd)) = getFuncPointer(symPtr, "runtime.itabAdd")

	_itabTable = *(*uintptr)(getVarPointer(symPtr, "runtime.itabTable"))
	itabTable = (*itabTableType)(unsafe.Pointer(_itabTable))

	_itabLock = *(*uintptr)(getVarPointer(symPtr, "runtime.itabLock"))
	itabLock = (*mutex)(unsafe.Pointer(&_itabLock))
}
