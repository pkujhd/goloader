//go:build go1.13 && !go1.19
// +build go1.13,!go1.19

package goloader

import (
	"unsafe"
)

const (
	_InitTaskSuffix = "..inittask"
)

func getInitFuncName(packagename string) string {
	return packagename + _InitTaskSuffix
}

// doInit is defined in package runtime
//go:linkname doInit runtime.doInit
func doInit(t unsafe.Pointer) // t should be a *runtime.initTask

func (linker *Linker) doInitialize(codeModule *CodeModule, symbolMap map[string]uintptr) error {
	for _, name := range linker.initFuncs {
		if funcPtr, ok := symbolMap[name]; ok {
			doInit(adduintptr(funcPtr, 0))
		}
	}
	return nil
}
