//go:build go1.21 && !go1.23
// +build go1.21,!go1.23

package goloader

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/reloctype"
)

type initTask struct {
	state uint32 // 0 = uninitialized, 1 = in progress, 2 = done
	nfns  uint32
	// followed by nfns pcs, uintptr sized, one per init function to run
}

const (
	_InitTaskSuffix = "..inittask"
)

func getInitFuncName(packagename string) string {
	return obj.PathToPrefix(packagename) + _InitTaskSuffix
}

//go:linkname doInit1 runtime.doInit1
func doInit1(t unsafe.Pointer) // t should be a *runtime.initTask

func (linker *Linker) doInitialize(symPtr, symbolMap map[string]uintptr) error {
	for _, pkg := range linker.pkgs {
		name := getInitFuncName(pkg.PkgPath)
		if ptr, ok := symbolMap[name]; ok {
			for _, loc := range linker.symMap[name].Reloc {
				if loc.Type == reloctype.R_INITORDER {
					if locPtr, ok := symPtr[loc.Sym.Name]; ok {
						doInit1(adduintptr(locPtr, 0))
					} else if locPtr, ok := symbolMap[loc.Sym.Name]; ok {
						doInit1(adduintptr(locPtr, 0))
					}
				}
			}
			task := *(*initTask)(adduintptr(ptr, 0))
			if task.nfns == 0 {
				continue
			}
			doInit1(adduintptr(ptr, 0))
		}
	}
	return nil
}
