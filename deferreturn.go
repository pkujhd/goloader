//go:build go1.14 && !go1.25
// +build go1.14,!go1.25

package goloader

import (
	"cmd/objfile/sys"
	"fmt"

	"github.com/pkujhd/goloader/objabi/dataindex"
)

func (linker *Linker) addDeferReturn(_func *_func, module *moduledata) (err error) {
	funcname := getfuncname(_func, module)
	Func := linker.SymMap[funcname].Func
	if Func != nil && len(Func.FuncData) > dataindex.FUNCDATA_OpenCodedDeferInfo {
		sym := linker.SymMap[funcname]
		for _, r := range sym.Reloc {
			if r.SymName == RuntimeDeferReturn {
				//../cmd/link/internal/ld/pcln.go:pclntab
				switch linker.Arch.Name {
				case sys.Arch386.Name, sys.ArchAMD64.Name:
					_func.Deferreturn = uint32(r.Offset) - uint32(sym.Offset) - 1
				case sys.ArchARM.Name, sys.ArchARM64.Name:
					_func.Deferreturn = uint32(r.Offset) - uint32(sym.Offset)
				default:
					err = fmt.Errorf("not support arch:%s", linker.Arch.Name)
				}
				break
			}
		}
	}
	return err
}
