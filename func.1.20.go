//go:build go1.20 && !go1.21
// +build go1.20,!go1.21

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
	nameoff  int32  // function name, as index into moduledata.funcnametab.

	args        int32  // in/out args size
	deferreturn uint32 // offset of start of a deferreturn call instruction from entry, if any.

	pcsp      uint32
	pcfile    uint32
	pcln      uint32
	npcdata   uint32
	cuOffset  uint32 // runtime.cutab offset of this function's CU
	startLine int32  // line number of start of function (func keyword/TEXT directive)
	funcID    funcID // set for certain special runtime functions
	flag      funcFlag
	_         [1]byte // pad
	nfuncdata uint8   // must be last, must end on a uint32-aligned boundary

	// The end of the struct is followed immediately by two variable-length
	// arrays that reference the pcdata and funcdata locations for this
	// function.

	// pcdata contains the offset into moduledata.pctab for the start of
	// that index's table. e.g.,
	// &moduledata.pctab[_func.pcdata[_PCDATA_UnsafePoint]] is the start of
	// the unsafe point table.
	//
	// An offset of 0 indicates that there is no table.
	//
	// pcdata [npcdata]uint32

	// funcdata contains the offset past moduledata.gofunc which contains a
	// pointer to that index's funcdata. e.g.,
	// *(moduledata.gofunc +  _func.funcdata[_FUNCDATA_ArgsPointerMaps]) is
	// the argument pointer map.
	//
	// An offset of ^uint32(0) indicates that there is no entry.
	//
	// funcdata [nfuncdata]uint32
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
		startLine:	 int32(symbol.Func.StartLine),
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
