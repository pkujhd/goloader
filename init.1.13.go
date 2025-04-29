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

func getInitFuncName(packageName string) string {
	return obj.PathToPrefix(packageName) + _InitTaskSuffix
}

func isNeedInitTaskInPlugin(name string) bool {
	return name == getInitFuncName(DefaultPkgPath)
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

func isCompleteInitialization(linker *Linker, name string, symPtr map[string]uintptr) bool {
	return true
}
