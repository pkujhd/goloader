//go:build go1.18 && !go1.25
// +build go1.18,!go1.25

package stackobject

import (
	"unsafe"
)

// A stackObjectRecord is generated by the compiler for each stack object in a stack frame.
// This record must match the generator code in cmd/compile/internal/liveness/plive.go:emitStackObjects.
type stackObjectRecord struct {
	// offset in frame
	// if negative, offset from varp
	// if non-negative, offset from argp
	off       int32
	size      int32
	_ptrdata  int32  // ptrdata, or -ptrdata is GC prog is used
	gcdataoff uint32 // offset to gcdata from moduledata.rodata
}

func setStackObjectPtr(obj *stackObjectRecord, ptr unsafe.Pointer, noptrdata uintptr) {
	obj.gcdataoff = uint32(uintptr(ptr) - noptrdata)
}
