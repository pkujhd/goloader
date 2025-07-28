//go:build go1.8 && !go1.18
// +build go1.8,!go1.18

package link

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
)

type functab struct {
	entry   uintptr
	funcoff uintptr
}

func initfunctab(entry, funcoff, text uintptr) functab {
	functabdata := functab{
		entry:   uintptr(entry),
		funcoff: uintptr(funcoff),
	}
	return functabdata
}

func addfuncdata(module *moduledata, Func *obj.Func, _func *_func) {
	funcdata := make([]uintptr, 0)
	for _, v := range Func.FuncData {
		if v != 0 {
			funcdata = append(funcdata, v+module.noptrdata)
		} else {
			funcdata = append(funcdata, v)
		}
	}
	grow(&module.pclntable, alignof(len(module.pclntable), PtrSize))
	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&funcdata[0])), PtrSize*int(_func.Nfuncdata))
}
