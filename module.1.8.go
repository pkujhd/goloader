//go:build go1.8 && !go1.10
// +build go1.8,!go1.10

package goloader

//go:linkname activeModules runtime.activeModules
func activeModules() []*moduledata

// pcHeader holds data used by the pclntab lookups.
type pcHeader struct {
	magic      uint32 // 0xFFFFFFFB
	pad1, pad2 uint8  // 0,0
	minLC      uint8  // min instruction size
	ptrSize    uint8  // size of a ptr in bytes
}

// moduledata records information about the layout of the executable
// image. It is written by the linker. Any changes here must be
// matched changes to the code in cmd/internal/ld/symtab.go:symtab.
// moduledata is stored in read-only memory; none of the pointers here
// are visible to the garbage collector.
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

	gcdatamask, gcbssmask bitvector

	typemap map[typeOff]*_type // offset to *_rtype in previous module

	next *moduledata
}

func initmodule(module *moduledata, linker *Linker) {
	module.filetab = linker.filetab
}
