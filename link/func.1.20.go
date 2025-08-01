//go:build go1.20 && !go1.26
// +build go1.20,!go1.26

package link

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
	Entryoff uint32 // start pc, as offset from link.text/pcHeader.textStart
	Nameoff  int32  // function name, as index into link.funcnametab.

	Args        int32  // in/out args size
	Deferreturn uint32 // offset of start of a deferreturn call instruction from entry, if any.

	Pcsp      uint32
	Pcfile    uint32
	Pcln      uint32
	Npcdata   uint32
	CuOffset  uint32 // runtime.cutab offset of this function's CU
	StartLine int32  // line number of start of function (func keyword/TEXT directive)
	FuncID    funcID // set for certain special runtime functions
	Flag      funcFlag
	_         [1]byte // pad
	Nfuncdata uint8   // must be last, must end on a uint32-aligned boundary

	// The end of the struct is followed immediately by two variable-length
	// arrays that reference the pcdata and funcdata locations for this
	// function.

	// pcdata contains the offset into link.pctab for the start of
	// that index's table. e.g.,
	// &link.pctab[_func.pcdata[_PCDATA_UnsafePoint]] is the start of
	// the unsafe point table.
	//
	// An offset of 0 indicates that there is no table.
	//
	// pcdata [npcdata]uint32

	// funcdata contains the offset past link.gofunc which contains a
	// pointer to that index's funcdata. e.g.,
	// *(link.gofunc +  _func.funcdata[_FUNCDATA_ArgsPointerMaps]) is
	// the argument pointer map.
	//
	// An offset of ^uint32(0) indicates that there is no entry.
	//
	// funcdata [nfuncdata]uint32
}

func initfunc(symbol *obj.ObjSymbol, nameOff, pcspOff, pcfileOff, pclnOff, cuOff int) _func {
	fdata := _func{
		Entryoff:    uint32(0),
		Nameoff:     int32(nameOff),
		Args:        int32(symbol.Func.Args),
		Deferreturn: uint32(0),
		Pcsp:        uint32(pcspOff),
		Pcfile:      uint32(pcfileOff),
		Pcln:        uint32(pclnOff),
		Npcdata:     uint32(len(symbol.Func.PCData)),
		CuOffset:    uint32(cuOff),
		StartLine:   int32(symbol.Func.StartLine),
		FuncID:      funcID(symbol.Func.FuncID),
		Flag:        funcFlag(symbol.Func.FuncFlag),
		Nfuncdata:   uint8(len(symbol.Func.FuncData)),
	}
	return fdata
}

func setfuncentry(f *_func, entry uintptr, text uintptr) {
	f.Entryoff = uint32(entry - text)
}

func getfuncentry(f *_func, text uintptr) uintptr {
	return text + uintptr(f.Entryoff)
}

func getfuncname(f *_func, md *moduledata) string {
	if f.Nameoff <= 0 || f.Nameoff >= int32(len(md.funcnametab)) {
		return EmptyString
	}
	return gostringnocopy(&(md.funcnametab[f.Nameoff]))
}

func getfuncID(f *_func) uint8 {
	return uint8(f.FuncID)
}

func adaptePCFile(linker *Linker, symbol *obj.ObjSymbol) {
}
