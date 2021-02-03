// +build go1.14
// +build !go1.17

package goloader

import (
	"cmd/objfile/sys"
	"fmt"
)

func (linker *Linker) _addDeferReturn(_func *_func) (err error) {
	funcname := gostringnocopy(&linker.pclntable[_func.nameoff])
	Func := linker.symMap[funcname].Func
	if Func != nil && len(Func.FuncData) > _FUNCDATA_OpenCodedDeferInfo {
		sym := linker.symMap[funcname]
		for _, r := range sym.Reloc {
			if r.Sym == linker.symMap[RuntimeDeferReturn] {
				//../cmd/link/internal/ld/pcln.go:pclntab
				switch linker.Arch {
				case sys.Arch386.Name, sys.ArchAMD64.Name:
					_func.deferreturn = uint32(r.Offset) - uint32(sym.Offset) - 1
				case sys.ArchARM.Name, sys.ArchARM64.Name:
					_func.deferreturn = uint32(r.Offset) - uint32(sym.Offset)
				default:
					err = fmt.Errorf("not support arch:%s", linker.Arch)
				}
				break
			}
		}
	}
	return err
}
