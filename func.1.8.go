//go:build go1.8 && !go1.12
// +build go1.8,!go1.12

package goloader

import "github.com/eh-steve/goloader/obj"

type _func struct {
	entry   uintptr // start pc
	nameoff int32   // function name

	args int32 // in/out args size
	_    int32 // previously legacy frame size; kept for layout compatibility

	pcsp      int32
	pcfile    int32
	pcln      int32
	npcdata   int32
	nfuncdata int32
}

func initfunc(symbol *obj.ObjSymbol, nameOff, spOff, pcfileOff, pclnOff, cuOff int) _func {
	fdata := _func{
		entry:     uintptr(0),
		nameoff:   int32(nameOff),
		args:      int32(symbol.Func.Args),
		pcsp:      int32(spOff),
		pcfile:    int32(pcfileOff),
		pcln:      int32(pclnOff),
		npcdata:   int32(len(symbol.Func.PCData)),
		nfuncdata: int32(len(symbol.Func.FuncData)),
	}
	return fdata
}

func setfuncentry(f *_func, entry uintptr, text uintptr) {
	f.entry = entry
}

func getfuncentry(f *_func, text uintptr) uintptr {
	return f.entry
}

func getfuncname(f *_func, md *moduledata) string {
	if f.nameoff <= 0 || f.nameoff >= int32(len(md.pclntable)) {
		return EmptyString
	}
	return gostringnocopy(&(md.pclntable[f.nameoff]))
}

func getfuncID(f *_func) uint8 {
	return 0
}
