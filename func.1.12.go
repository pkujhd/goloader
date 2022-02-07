//go:build go1.12 && !go1.16
// +build go1.12,!go1.16

package goloader

import (
	"cmd/objfile/objabi"
	"github.com/pkujhd/goloader/obj"
	"strings"
)

// A funcID identifies particular functions that need to be treated
// specially by the runtime.
// Note that in some situations involving plugins, there may be multiple
// copies of a particular special runtime function.
// Note: this list must match the list in cmd/internal/objabi/funcid.go.
type funcID uint8

// Layout of in-memory per-function information prepared by linker
// See https://golang.org/s/go12symtab.
// Keep in sync with linker (../cmd/link/internal/ld/pcln.go:/pclntab)
// and with package debug/gosym and with symtab.go in package runtime.
type _func struct {
	entry   uintptr // start pc
	nameoff int32   // function name

	args        int32  // in/out args size
	deferreturn uint32 // offset of start of a deferreturn call instruction from entry, if any.

	pcsp      int32
	pcfile    int32
	pcln      int32
	npcdata   int32
	funcID    funcID  // set for certain special runtime functions
	_         [2]int8 // unused
	nfuncdata uint8   // must be last
}

func initfunc(symbol *obj.ObjSymbol, nameOff, spOff, pcfileOff, pclnOff int, cuOff int) _func {
	fdata := _func{
		entry:       uintptr(0),
		nameoff:     int32(nameOff),
		args:        int32(symbol.Func.Args),
		deferreturn: uint32(0),
		pcsp:        int32(spOff),
		pcfile:      int32(pcfileOff),
		pcln:        int32(pclnOff),
		npcdata:     int32(len(symbol.Func.PCData)),
		funcID:      funcID(objabi.GetFuncID(symbol.Name, strings.TrimPrefix(symbol.Func.File[0], FileSymPrefix))),
		nfuncdata:   uint8(len(symbol.Func.FuncData)),
	}
	return fdata
}

func setfuncentry(f *_func, entry uintptr, text uintptr) {
	f.entry = entry
}
