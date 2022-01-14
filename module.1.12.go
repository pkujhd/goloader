//go:build go1.12 && !go1.13
// +build go1.12,!go1.13

package goloader

import (
	"cmd/objfile/objabi"
	"strings"
	"unsafe"
)

type functab struct {
	entry   uintptr
	funcoff uintptr
}

// PCDATA and FUNCDATA table indexes.
//
// See funcdata.h and ../cmd/internal/objabi/funcdata.go.
const (
	_PCDATA_StackMapIndex       = 0
	_PCDATA_InlTreeIndex        = 1
	_PCDATA_RegMapIndex         = 2
	_FUNCDATA_ArgsPointerMaps   = 0
	_FUNCDATA_LocalsPointerMaps = 1
	_FUNCDATA_InlTree           = 2
	_FUNCDATA_RegPointerMaps    = 3
	_FUNCDATA_StackObjects      = 4
	_ArgsSizeUnknown            = -0x80000000
)

type moduledata struct {
	pclntable    []byte
	ftab         []functab
	filetab      []uint32
	findfunctab  uintptr
	minpc, maxpc uintptr

	text, etext           uintptr
	noptrdata, enoptrdata uintptr
	data, edata           uintptr
	bss, ebss             uintptr
	noptrbss, enoptrbss   uintptr
	end, gcdata, gcbss    uintptr
	types, etypes         uintptr

	textsectmap []textsect
	typelinks   []int32 // offsets from types
	itablinks   []*itab

	ptab []ptabEntry

	pluginpath string
	pkghashes  []modulehash

	modulename   string
	modulehashes []modulehash

	hasmain uint8 // 1 if module contains the main function, 0 otherwise

	gcdatamask, gcbssmask bitvector

	typemap map[typeOff]uintptr // offset to *_rtype in previous module

	bad bool // module failed to load and should be ignored

	next *moduledata
}

// A funcID identifies particular functions that need to be treated
// specially by the runtime.
// Note that in some situations involving plugins, there may be multiple
// copies of a particular special runtime function.
// Note: this list must match the list in cmd/internal/objabi/funcid.go.
type funcID uint8

type _func struct {
	entry   uintptr // start pc
	nameoff int32   // function name

	args int32 // in/out args size
	_    int32 // previously legacy frame size; kept for layout compatibility

	pcsp      int32
	pcfile    int32
	pcln      int32
	npcdata   int32
	funcID    funcID  // set for certain special runtime functions
	_         [2]int8 // unused
	nfuncdata uint8
}

func init_func(symbol *ObjSymbol, nameOff, spOff, pcfileOff, pclnOff, cuOff int) _func {
	fdata := _func{
		entry:     uintptr(0),
		nameoff:   int32(nameOff),
		args:      int32(symbol.Func.Args),
		pcsp:      int32(spOff),
		pcfile:    int32(pcfileOff),
		pcln:      int32(pclnOff),
		npcdata:   int32(len(symbol.Func.PCData)),
		funcID:    funcID(objabi.GetFuncID(symbol.Name, strings.TrimPrefix(symbol.Func.File[0], FileSymPrefix))),
		nfuncdata: uint8(len(symbol.Func.FuncData)),
	}
	return fdata
}

func initfunctab(entry, funcoff, text uintptr) functab {
	functabdata := functab{
		entry:   uintptr(entry),
		funcoff: uintptr(funcoff),
	}
	return functabdata
}

func setfuncentry(f *_func, entry uintptr, text uintptr) {
	f.entry = entry
}

func addfuncdata(module *moduledata, Func *Func, _func *_func) {
	funcdata := make([]uintptr, 0)
	for _, v := range Func.FuncData {
		if v != 0 {
			funcdata = append(funcdata, v+module.noptrdata)
		} else {
			funcdata = append(funcdata, v)
		}
	}
	grow(&module.pclntable, alignof(len(module.pclntable), PtrSize))
	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&funcdata[0])), PtrSize*int(_func.nfuncdata))
}
