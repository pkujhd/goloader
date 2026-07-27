//go:build go1.10 && !go1.23
// +build go1.10,!go1.23

package link

import (
	"unsafe"
)

//go:linkname _itabTable runtime.itabTable
var _itabTable uintptr // pointer to current table

var itabTable *itabTableType = (*itabTableType)(unsafe.Pointer(_itabTable))

//go:linkname _itabLock runtime.itabLock
var _itabLock uintptr

var itabLock *mutex = (*mutex)(unsafe.Pointer(&_itabLock))

//go:linkname itabAdd runtime.itabAdd
func itabAdd(m *itab)
