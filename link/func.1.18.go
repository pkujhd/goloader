//go:build go1.18 && !go1.20
// +build go1.18,!go1.20

package link

import (
	"github.com/pkujhd/goloader/constants"
	"github.com/pkujhd/goloader/obj"
)

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
	Nameoff  int32  // function name

	Args        int32  // in/out args size
	Deferreturn uint32 // offset of start of a deferreturn call instruction from entry, if any.

	Pcsp      uint32
	Pcfile    uint32
	Pcln      uint32
	Npcdata   uint32
	CuOffset  uint32 // runtime.cutab offset of this function's CU
	FuncID    funcID // set for certain special runtime functions
	Flag      funcFlag
	_         [1]byte // pad
	Nfuncdata uint8   // must be last, must end on a uint32-aligned boundary
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
		return constants.EmptyString
	}
	return gostringnocopy(&(md.funcnametab[f.Nameoff]))
}

func getfuncID(f *_func) uint8 {
	return uint8(f.FuncID)
}
