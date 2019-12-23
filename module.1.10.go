// +build go1.10 go1.11
// +build !go1.12,!go1.13

package goloader

import (
	"fmt"
	"strings"
)

// layout of Itab known to compilers
// allocated in non-garbage-collected memory
// Needs to be in sync with
// ../cmd/compile/internal/gc/reflect.go:/^func.dumptypestructs.
type itab struct {
	inter  uintptr
	_type  uintptr
	link   uintptr
	hash   uint32 // copy of _type.hash. Used for type switches.
	bad    bool   // type does not implement interface
	inhash bool   // has this itab been added to hash?
	unused [2]byte
	fun    [1]uintptr // variable sized
}

// PCDATA and FUNCDATA table indexes.
//
// See funcdata.h and ../cmd/internal/obj/funcdata.go.
const (
	_PCDATA_StackMapIndex       = 0
	_PCDATA_InlTreeIndex        = 1
	_FUNCDATA_ArgsPointerMaps   = 0
	_FUNCDATA_LocalsPointerMaps = 1
	_FUNCDATA_InlTree           = 2
	_ArgsSizeUnknown            = -0x80000000
)

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

	hasmain uint8 // 1 if module contains the main function, 0 otherwise

	gcdatamask, gcbssmask bitvector

	typemap map[typeOff]uintptr // offset to *_rtype in previous module

	bad bool // module failed to load and should be ignored

	next *moduledata
}

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

func readFuncData(module *Module, curSymFile symFile,
	allSyms map[string]symFile, gcObjs map[string]uintptr,
	fileTabOffsetMap map[string]int, curSymOffset, curCodeLen int) {

	fs := readAtSeeker{ReadSeeker: curSymFile.file}
	curSym := curSymFile.sym

	{
		x := curCodeLen
		b := x / pcbucketsize
		i := x % pcbucketsize / (pcbucketsize / nsub)
		for lb := b - len(module.pcfunc); lb >= 0; lb-- {
			module.pcfunc = append(module.pcfunc, findfuncbucket{
				idx: uint32(256 * len(module.pcfunc))})
		}
		bucket := &module.pcfunc[b]
		bucket.subbuckets[i] = byte(len(module.ftab) - int(bucket.idx))
	}

	var fileTabOffset = len(module.filetab)
	var fileOffsets []uint32
	var fullFile string
	for _, fileName := range curSym.Func.File {
		fileOffsets = append(fileOffsets, uint32(len(fullFile)+len(module.pclntable)))
		fileName = strings.TrimLeft(curSym.Func.File[0], "gofile..")
		fullFile += fileName + "\x00"
	}
	if tabOffset, ok := fileTabOffsetMap[fullFile]; !ok {
		module.pclntable = append(module.pclntable, []byte(fullFile)...)
		fileTabOffsetMap[fullFile] = fileTabOffset
		module.filetab = append(module.filetab, fileOffsets...)
	} else {
		fileTabOffset = tabOffset
	}
	var pcFileHead [2]byte
	if fileTabOffset > 128 {
		fmt.Println("filetab overflow!")
	}
	pcFileHead[0] = byte(fileTabOffset << 1)

	nameOff := len(module.pclntable)
	nameByte := make([]byte, len(curSym.Name)+1)
	copy(nameByte, []byte(curSym.Name))
	module.pclntable = append(module.pclntable, nameByte...)

	spOff := len(module.pclntable)
	var fb = make([]byte, curSym.Func.PCSP.Size)
	fs.ReadAt(fb, curSym.Func.PCSP.Offset)
	// fmt.Println("sp val:", fb)
	module.pclntable = append(module.pclntable, fb...)

	pcfileOff := len(module.pclntable)
	fb = make([]byte, curSym.Func.PCFile.Size)
	fs.ReadAt(fb, curSym.Func.PCFile.Offset)
	// dumpPCData(fb, "pcfile")
	module.pclntable = append(module.pclntable, pcFileHead[:]...)
	module.pclntable = append(module.pclntable, fb...)

	pclnOff := len(module.pclntable)
	fb = make([]byte, curSym.Func.PCLine.Size)
	fs.ReadAt(fb, curSym.Func.PCLine.Offset)
	module.pclntable = append(module.pclntable, fb...)

	fdata := _func{
		entry:     uintptr(curSymOffset),
		nameoff:   int32(nameOff),
		args:      int32(curSym.Func.Args),
		pcsp:      int32(spOff),
		pcfile:    int32(pcfileOff),
		pcln:      int32(pclnOff),
		npcdata:   int32(len(curSym.Func.PCData)),
		nfuncdata: int32(len(curSym.Func.FuncData)),
	}
	var fInfo funcInfoData
	fInfo._func = fdata
	for _, data := range curSym.Func.PCData {
		fInfo.pcdata = append(fInfo.pcdata, uint32(len(module.pclntable)))

		var b = make([]byte, data.Size)
		fs.ReadAt(b, data.Offset)
		// dumpPCData(b)
		module.pclntable = append(module.pclntable, b...)
	}
	for _, data := range curSym.Func.FuncData {
		var offset uintptr
		if off, ok := gcObjs[data.Sym.Name]; !ok {
			if gcobj, ok := allSyms[data.Sym.Name]; ok {
				var b = make([]byte, gcobj.sym.Data.Size)
				cfs := readAtSeeker{ReadSeeker: gcobj.file}
				cfs.ReadAt(b, gcobj.sym.Data.Offset)
				offset = uintptr(len(module.stkmaps))
				module.stkmaps = append(module.stkmaps, b)
				gcObjs[data.Sym.Name] = offset
			} else if len(data.Sym.Name) == 0 {
				fInfo.funcdata = append(fInfo.funcdata, 0)
			} else {
				fmt.Println("unknown gcobj:", data.Sym.Name)
			}
		} else {
			offset = off
		}

		fInfo.funcdata = append(fInfo.funcdata, offset)
	}

	module.ftab = append(module.ftab, functab{
		entry: uintptr(curSymOffset),
	})

	module.funcinfo = append(module.funcinfo, fInfo)
}
