//go:build go1.13 && !go1.20
// +build go1.13,!go1.20

package goloader

import (
	"strings"
	"unsafe"
)

const (
	_InitTaskSuffix = "..inittask"
)

func getInitFuncName(packagename string) string {
	return packagename + _InitTaskSuffix
}

// doInit is defined in package runtime
//
//go:linkname doInit runtime.doInit
func doInit(t unsafe.Pointer) // t should be a *runtime.initTask

type initTask struct {
	state uintptr // 0 = uninitialized, 1 = in progress, 2 = done
	ndeps uintptr
	nfns  uintptr
}

func (linker *Linker) doInitialize(codeModule *CodeModule, symbolMap map[string]uintptr) error {
	for _, name := range linker.initFuncs {
		if taskPtr, ok := symbolMap[name]; ok {
			shouldSkipDedup := false
			for _, pkgPath := range linker.options.SkipTypeDeduplicationForPackages {
				if strings.HasPrefix(name, pkgPath) {
					shouldSkipDedup = true
				}
			}
			if shouldSkipDedup {
				x := (*initTask)(unsafe.Pointer(taskPtr))
				x.state = 0 // Reset the inittask state in order to rerun the init function for the new version of the package
			}
			doInit(adduintptr(taskPtr, 0))
		}
	}
	return nil
}
