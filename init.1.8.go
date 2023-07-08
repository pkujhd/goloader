//go:build go1.8 && !go1.13
// +build go1.8,!go1.13

package goloader

import (
	"unsafe"
)

const (
	_InitTaskSuffix = ".init"
)

func getInitFuncName(packagename string) string {
	return packagename + _InitTaskSuffix
}

func (linker *Linker) doInitialize(symPtr, symbolMap map[string]uintptr) error {
	for _, name := range linker.initFuncs {
		if funcPtr, ok := symbolMap[name]; ok {
			funcPtrContainer := (uintptr)(unsafe.Pointer(&funcPtr))
			runFunc := *(*func())(unsafe.Pointer(&funcPtrContainer))
			runFunc()
		}
	}
	return nil
}
