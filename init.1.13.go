//go:build go1.13 && !go1.21
// +build go1.13,!go1.21

package goloader

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
)

const (
	_InitTaskSuffix = "..inittask"
)

func getInitFuncName(packagename string) string {
	return obj.PathToPrefix(packagename) + _InitTaskSuffix
}

//go:linkname doInit runtime.doInit
func doInit(t unsafe.Pointer) // t should be a *runtime.initTask

func (linker *Linker) doInitialize(symPtr, symbolMap map[string]uintptr) error {
	for _, pkg := range linker.Packages {
		name := getInitFuncName(pkg.PkgPath)
		if funcPtr, ok := symbolMap[name]; ok {
			doInit(adduintptr(funcPtr, 0))
		}
	}
	return nil
}
