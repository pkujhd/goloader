//go:build go1.21 && !go1.22
// +build go1.21,!go1.22

package goloader

import (
	"cmd/objfile/objabi"
	"slices"
	"sort"
	"strings"
	"unsafe"
)

const (
	_InitTaskSuffix = "..inittask"
)

func getInitFuncName(packagename string) string {
	return objabi.PathToPrefix(packagename) + _InitTaskSuffix
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
	// Autolib order is not necessarily the same as the  (*Link).inittaskSym algorithm in cmd/link/internal/ld/inittask.go,
	// but it works and avoids a Kahn's graph traversal of R_INITORDER relocs...
	autolibOrder := linker.Autolib()
	for i := range autolibOrder {
		// ..inittask symbol names will have their package escaped, so autolib list needs to as well
		autolibOrder[i] = objabi.PathToPrefix(autolibOrder[i])
	}
	sort.Slice(linker.initFuncs, func(i, j int) bool {
		return slices.Index(autolibOrder, strings.TrimSuffix(linker.initFuncs[i], _InitTaskSuffix)) < slices.Index(autolibOrder, strings.TrimSuffix(linker.initFuncs[j], _InitTaskSuffix))
	})
	for _, name := range linker.initFuncs {
		if taskPtr, ok := symbolMap[name]; ok && taskPtr != 0 { // taskPtr may be nil if the inittask wasn't seen in the host symtab (probably a no-op and therefore eliminated)
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
				// Linker is expected to have stripped inittasks with no funcs
				continue
			}
			doInit1(adduintptr(taskPtr, 0))
		}
	}
	return nil
}
