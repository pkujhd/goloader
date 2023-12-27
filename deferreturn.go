//go:build go1.14 && !go1.23
// +build go1.14,!go1.23

package goloader

import (
	"cmd/objfile/sys"
	"fmt"

	"github.com/pkujhd/goloader/objabi/dataindex"
)

func (linker *Linker) addDeferReturn(_func *_func, module *moduledata) (err error) {
	funcname := getfuncname(_func, module)
	Func := linker.symMap[funcname].Func
	if Func != nil && len(Func.FuncData) > dataindex.FUNCDATA_OpenCodedDeferInfo {
		sym := linker.symMap[funcname]
		for _, r := range sym.Reloc {
			if r.Sym.Name == RuntimeDeferReturn {
				//../cmd/link/internal/ld/pcln.go:pclntab
				switch linker.arch.Name {
				case sys.Arch386.Name, sys.ArchAMD64.Name:
					_func.deferreturn = uint32(r.Offset) - uint32(sym.Offset) - 1
				case sys.ArchARM.Name, sys.ArchARM64.Name:
					_func.deferreturn = uint32(r.Offset) - uint32(sym.Offset)
				default:
					err = fmt.Errorf("not support arch:%s", linker.arch.Name)
				}
				break
			}
		}
	}
	return err
}
