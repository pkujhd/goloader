//go:build go1.18 && !go1.19
// +build go1.18,!go1.19

package goloader

import (
	"unsafe"
)

type functab struct {
	entry   uint32
	funcoff uint32
}

// PCDATA and FUNCDATA table indexes.
//
// See funcdata.h and ../cmd/internal/objabi/funcdata.go.
const (
	_PCDATA_UnsafePoint   = 0
	_PCDATA_StackMapIndex = 1
	_PCDATA_InlTreeIndex  = 2
	_PCDATA_ArgLiveIndex  = 3

	_FUNCDATA_ArgsPointerMaps    = 0
	_FUNCDATA_LocalsPointerMaps  = 1
	_FUNCDATA_StackObjects       = 2
	_FUNCDATA_InlTree            = 3
	_FUNCDATA_OpenCodedDeferInfo = 4
	_FUNCDATA_ArgInfo            = 5
	_FUNCDATA_ArgLiveInfo        = 6

	_ArgsSizeUnknown = -0x80000000
)

// pcHeader holds data used by the pclntab lookups.
type pcHeader struct {
	magic          uint32  // 0xFFFFFFF0
	pad1, pad2     uint8   // 0,0
	minLC          uint8   // min instruction size
	ptrSize        uint8   // size of a ptr in bytes
	nfunc          int     // number of functions in the module
	nfiles         uint    // number of entries in the file tab
	textStart      uintptr // base for function entry PC offsets in this module, equal to moduledata.text
	funcnameOffset uintptr // offset to the funcnametab variable from pcHeader
	cuOffset       uintptr // offset to the cutab variable from pcHeader
	filetabOffset  uintptr // offset to the filetab variable from pcHeader
	pctabOffset    uintptr // offset to the pctab variable from pcHeader
	pclnOffset     uintptr // offset to the pclntab variable from pcHeader
}

// moduledata records information about the layout of the executable
// image. It is written by the linker. Any changes here must be
// matched changes to the code in cmd/link/internal/ld/symtab.go:symtab.
// moduledata is stored in statically allocated non-pointer memory;
// none of the pointers here are visible to the garbage collector.
type moduledata struct {
	pcHeader     *pcHeader
	funcnametab  []byte
	cutab        []uint32
	filetab      []byte
	pctab        []byte
	pclntable    []byte
	ftab         []functab
	findfunctab  uintptr
	minpc, maxpc uintptr

	text, etext           uintptr
	noptrdata, enoptrdata uintptr
	data, edata           uintptr
	bss, ebss             uintptr
	noptrbss, enoptrbss   uintptr
	end, gcdata, gcbss    uintptr
	types, etypes         uintptr
	rodata                uintptr
	gofunc                uintptr // go.func.*

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

func init_func(symbol *ObjSymbol, nameOff, spOff, pcfileOff, pclnOff, cuOff int) _func {
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

func initfunctab(entry, funcoff, text uintptr) functab {
	functabdata := functab{
		entry:   uint32(entry - text),
		funcoff: uint32(funcoff),
	}
	return functabdata
}

func setfuncentry(f *_func, entry uintptr, text uintptr) {
	f.entryoff = uint32(entry - text)
}

func addfuncdata(module *moduledata, Func *Func, _func *_func) {
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
