//go:build go1.21 && !go1.22
// +build go1.21,!go1.22

package goloader

import (
	"slices"
	"sort"
	"strings"
	"unsafe"
)

const (
	_InitTaskSuffix = "..inittask"
)

func getInitFuncName(packagename string) string {
	return packagename + _InitTaskSuffix
}

// doInit1 is defined in package runtime
//
//go:linkname doInit1 runtime.doInit1
func doInit1(t unsafe.Pointer) // t should be a *runtime.initTask

type initTask struct {
	state uint32 // 0 = uninitialized, 1 = in progress, 2 = done
	nfns  uint32
	// followed by nfns pcs, uintptr sized, one per init function to run
}

func (linker *Linker) doInitialize(codeModule *CodeModule, symbolMap map[string]uintptr) error {
	autolibOrder := linker.Autolib()
	sort.Slice(linker.initFuncs, func(i, j int) bool {
		return slices.Index(autolibOrder, linker.initFuncs[i]) < slices.Index(autolibOrder, linker.initFuncs[j])
	})
	for _, name := range linker.initFuncs {
		if taskPtr, ok := symbolMap[name]; ok {
			shouldSkipDedup := false
			for _, pkgPath := range linker.options.SkipTypeDeduplicationForPackages {
				if strings.HasPrefix(name, pkgPath) {
					shouldSkipDedup = true
				}
			}
			x := (*initTask)(unsafe.Pointer(taskPtr))
			if shouldSkipDedup {
				x.state = 0 // Reset the inittask state in order to rerun the init function for the new version of the package
			}
			if x.nfns == 0 {
				continue
			}
			doInit1(adduintptr(taskPtr, 0))
		}
	}
	return nil
}
