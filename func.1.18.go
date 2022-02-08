//go:build go1.18 && !go1.19
// +build go1.18,!go1.19

package goloader

import "github.com/pkujhd/goloader/obj"

// A funcID identifies particular functions that need to be treated
// specially by the runtime.
// Note that in some situations involving plugins, there may be multiple
// copies of a particular special runtime function.
// Note: this list must match the list in cmd/internal/objabi/funcid.go.
type funcID uint8

// A FuncFlag holds bits about a function.
// This list must match the list in cmd/internal/objabi/funcid.go.
type funcFlag uint8

// Layout of in-memory per-function information prepared by linker
// See https://golang.org/s/go12symtab.
// Keep in sync with linker (../cmd/link/internal/ld/pcln.go:/pclntab)
// and with package debug/gosym and with symtab.go in package runtime.
type _func struct {
	entryoff uint32 // start pc, as offset from moduledata.text/pcHeader.textStart
	nameoff  int32  // function name

	args        int32  // in/out args size
	deferreturn uint32 // offset of start of a deferreturn call instruction from entry, if any.

	pcsp      uint32
	pcfile    uint32
	pcln      uint32
	npcdata   uint32
	cuOffset  uint32 // runtime.cutab offset of this function's CU
	funcID    funcID // set for certain special runtime functions
	flag      funcFlag
	_         [1]byte // pad
	nfuncdata uint8   // must be last, must end on a uint32-aligned boundary
}

func initfunc(symbol *obj.ObjSymbol, nameOff, spOff, pcfileOff, pclnOff, cuOff int) _func {
	fdata := _func{
		entryoff:    uint32(0),
		nameoff:     int32(nameOff),
		args:        int32(symbol.Func.Args),
		deferreturn: uint32(0),
		pcsp:        uint32(spOff),
		pcfile:      uint32(pcfileOff),
		pcln:        uint32(pclnOff),
		npcdata:     uint32(len(symbol.Func.PCData)),
		cuOffset:    uint32(cuOff),
		funcID:      funcID(symbol.Func.FuncID),
		flag:        funcFlag(symbol.Func.FuncFlag),
		nfuncdata:   uint8(len(symbol.Func.FuncData)),
	}
	return fdata
}

func setfuncentry(f *_func, entry uintptr, text uintptr) {
	f.entryoff = uint32(entry - text)
}

func getfuncentry(f *_func, text uintptr) uintptr {
	return text + uintptr(f.entryoff)
}

func getfuncname(f *_func, md *moduledata) string {
	if f.nameoff <= 0 || f.nameoff >= int32(len(md.funcnametab)) {
		return EmptyString
	}
	return gostringnocopy(&(md.funcnametab[f.nameoff]))
}

func getfuncID(f *_func) uint8 {
	return uint8(f.funcID)
}
