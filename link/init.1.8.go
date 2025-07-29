//go:build go1.8 && !go1.13
// +build go1.8,!go1.13

package link

import (
	"unsafe"

	"github.com/pkujhd/goloader/constants"
	"github.com/pkujhd/goloader/obj"
)

const (
	_InitTaskSuffix = ".init"
)

func getInitFuncName(packageName string) string {
	return obj.PathToPrefix(packageName) + _InitTaskSuffix
}

func isNeedInitTaskInPlugin(name string) bool {
	return name == getInitFuncName(constants.DefaultPkgPath)
}

func (linker *Linker) doInitialize(symPtr, symbolMap map[string]uintptr) error {
	for _, pkg := range linker.Packages {
		name := getInitFuncName(pkg.PkgPath)
		if funcPtr, ok := symbolMap[name]; ok {
			funcPtrContainer := (uintptr)(unsafe.Pointer(&funcPtr))
			runFunc := *(*func())(unsafe.Pointer(&funcPtrContainer))
			runFunc()
		}
	}
	return nil
}

func isCompleteInitialization(linker *Linker, name string, symPtr map[string]uintptr) bool {
	return true
}
