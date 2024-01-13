//go:build go1.8 && !go1.12
// +build go1.8,!go1.12

package goloader

import "github.com/pkujhd/goloader/obj"

type _func struct {
	Entry   uintptr // start pc
	Nameoff int32   // function name

	Args int32 // in/out args size
	_    int32 // previously legacy frame size; kept for layout compatibility

	Pcsp      int32
	Pcfile    int32
	Pcln      int32
	Npcdata   int32
	Nfuncdata int32
}

func initfunc(symbol *obj.ObjSymbol, nameOff, pcspOff, pcfileOff, pclnOff, cuOff int) _func {
	fdata := _func{
		Entry:     uintptr(0),
		Nameoff:   int32(nameOff),
		Args:      int32(symbol.Func.Args),
		Pcsp:      int32(pcspOff),
		Pcfile:    int32(pcfileOff),
		Pcln:      int32(pclnOff),
		Npcdata:   int32(len(symbol.Func.PCData)),
		Nfuncdata: int32(len(symbol.Func.FuncData)),
	}
	return fdata
}

func setfuncentry(f *_func, entry uintptr, text uintptr) {
	f.Entry = entry
}

func getfuncentry(f *_func, text uintptr) uintptr {
	return f.Entry
}

func getfuncname(f *_func, md *moduledata) string {
	if f.Nameoff <= 0 || f.Nameoff >= int32(len(md.pclntable)) {
		return EmptyString
	}
	return gostringnocopy(&(md.pclntable[f.Nameoff]))
}

func getfuncID(f *_func) uint8 {
	return 0
}

func adaptePCFile(linker *Linker, symbol *obj.ObjSymbol) {
	rewritePCFile(symbol, linker)
}
