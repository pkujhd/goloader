//go:build go1.16 && !go1.25
// +build go1.16,!go1.25

package goloader

import (
	"unsafe"
)

func updateFuncnameTabInUnix(md *moduledata, baseAddr uintptr, pclntabSectData []byte) {
	ptr := uintptr(unsafe.Pointer(&pclntabSectData[uint64(uintptr(unsafe.Pointer(&md.funcnametab[0]))-baseAddr)]))
	exeData.md.funcnametab = md.funcnametab
	(*sliceHeader)((unsafe.Pointer)(&exeData.md.funcnametab)).Data = ptr
}

func updateFuncnameTabInPe(md *moduledata, off uintptr) {
	ptr := off + uintptr(unsafe.Pointer(&md.funcnametab[0]))
	exeData.md.funcnametab = md.funcnametab
	(*sliceHeader)((unsafe.Pointer)(&exeData.md.funcnametab)).Data = ptr
}
