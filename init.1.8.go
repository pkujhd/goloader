//go:build go1.8 && !go1.13
// +build go1.8,!go1.13

package goloader

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
)

const (
	_InitTaskSuffix = ".init"
)

func getInitFuncName(packagename string) string {
	return obj.PathToPrefix(packagename) + _InitTaskSuffix
}

func (linker *Linker) doInitialize(symPtr, symbolMap map[string]uintptr) error {
	for _, pkg := range linker.pkgs {
		name := getInitFuncName(pkg.PkgPath)
		if funcPtr, ok := symbolMap[name]; ok {
			funcPtrContainer := (uintptr)(unsafe.Pointer(&funcPtr))
			runFunc := *(*func())(unsafe.Pointer(&funcPtrContainer))
			runFunc()
		}
	}
	return nil
}
