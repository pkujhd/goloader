//go:build go1.18 && !go1.20
// +build go1.18,!go1.20

package goloader

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
)

type functab struct {
	entry   uint32
	funcoff uint32
}

func initfunctab(entry, funcoff, text uintptr) functab {
	functabdata := functab{
		entry:   uint32(entry - text),
		funcoff: uint32(funcoff),
	}
	return functabdata
}

func addfuncdata(module *moduledata, Func *obj.Func, _func *_func) {
	funcdata := make([]uint32, 0)
	for _, v := range Func.FuncData {
		if v != 0 {
			funcdata = append(funcdata, (uint32)(v))
		} else {
			funcdata = append(funcdata, ^uint32(0))
		}
	}
	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&funcdata[0])), Uint32Size*int(_func.nfuncdata))
}
