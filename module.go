package goloader

import (
	"cmd/objfile/goobj"
	"encoding/binary"
	"fmt"
	"strings"
	"unsafe"
)

//go:linkname firstmoduledata runtime.firstmoduledata
var firstmoduledata moduledata

const PtrSize = 4 << (^uintptr(0) >> 63)
const Uint32Size = int(unsafe.Sizeof(uint32(0)))
const IntSize = int(unsafe.Sizeof(int(0)))
const _funcSize = int(unsafe.Sizeof(_func{}))

type functab struct {
	entry   uintptr
	funcoff uintptr
}

// findfunctab is an array of these structures.
// Each bucket represents 4096 bytes of the text segment.
// Each subbucket represents 256 bytes of the text segment.
// To find a function given a pc, locate the bucket and subbucket for
// that pc. Add together the idx and subbucket value to obtain a
// function index. Then scan the functab array starting at that
// index to find the target function.
// This table uses 20 bytes for every 4096 bytes of code, or ~0.5% overhead.
type findfuncbucket struct {
	idx        uint32
	subbuckets [16]byte
}

// Mapping information for secondary text sections
type textsect struct {
	vaddr    uintptr // prelinked section vaddr
	length   uintptr // section length
	baseaddr uintptr // relocated section address
}

type nameOff int32
type typeOff int32
type textOff int32

// A ptabEntry is generated by the compiler for each exported function
// and global variable in the main package of a plugin. It is used to
// initialize the plugin module's symbol map.
type ptabEntry struct {
	name nameOff
	typ  typeOff
}

type modulehash struct {
	modulename   string
	linktimehash string
	runtimehash  *string
}

type bitvector struct {
	n        int32 // # of bits
	bytedata *uint8
}

type funcInfoData struct {
	_func
	pcdata      []uint32
	funcdata    []uintptr
	stkobjReloc []goobj.Reloc
	Var         []goobj.Var
	name        string
}

type stackmap struct {
	n        int32   // number of bitmaps
	nbit     int32   // number of bits in each bitmap
	bytedata [1]byte // bitmaps, each starting on a byte boundary
}

type Module struct {
	pclntable []byte
	pcfunc    []findfuncbucket
	funcinfo  []funcInfoData
	ftab      []functab // entry need reloc
	filetab   []uint32
	stkmaps   [][]byte
}

const minfunc = 16                 // minimum function size
const pcbucketsize = 256 * minfunc // size of bucket in the pc->func lookup table
const nsub = len(findfuncbucket{}.subbuckets)

//go:linkname step runtime.step
func step(p []byte, pc *uintptr, val *int32, first bool) (newp []byte, ok bool)

//go:linkname findfunc runtime.findfunc
func findfunc(pc uintptr) funcInfo

//go:linkname funcdata runtime.funcdata
func funcdata(f funcInfo, i int32) unsafe.Pointer

//go:linkname funcname runtime.funcname
func funcname(f funcInfo) string

//go:linkname moduledataverify1 runtime.moduledataverify1
func moduledataverify1(datap *moduledata)

type funcInfo struct {
	*_func
	datap *moduledata
}

func readFuncData(reloc *CodeReloc, symName string, objsymmap map[string]objSym, curCodeLen int) {
	module := &reloc.Mod
	fs := readAtSeeker{ReadSeeker: objsymmap[symName].file}
	curSym := objsymmap[symName].sym

	x := curCodeLen
	b := x / pcbucketsize
	i := x % pcbucketsize / (pcbucketsize / nsub)
	for lb := b - len(module.pcfunc); lb >= 0; lb-- {
		module.pcfunc = append(module.pcfunc, findfuncbucket{
			idx: uint32(256 * len(module.pcfunc))})
	}
	bucket := &module.pcfunc[b]
	bucket.subbuckets[i] = byte(len(module.ftab) - int(bucket.idx))

	pcFileHead := make([]byte, 32)
	pcFileHeadSize := binary.PutUvarint(pcFileHead, uint64(len(module.filetab))<<1)
	for _, fileName := range curSym.Func.File {
		fileName = strings.TrimLeft(fileName, "gofile..") + "\x00"
		if off, ok := reloc.FileMap[fileName]; !ok {
			module.filetab = append(module.filetab, (uint32)(len(module.pclntable)))
			module.pclntable = append(module.pclntable, []byte(fileName)...)
		} else {
			module.filetab = append(module.filetab, uint32(off))
		}
	}

	nameOff := len(module.pclntable)
	nameByte := make([]byte, len(curSym.Name)+1)
	copy(nameByte, []byte(curSym.Name))
	module.pclntable = append(module.pclntable, nameByte...)

	spOff := len(module.pclntable)
	var fb = make([]byte, curSym.Func.PCSP.Size)
	fs.ReadAt(fb, curSym.Func.PCSP.Offset)
	module.pclntable = append(module.pclntable, fb...)

	pcfileOff := len(module.pclntable)
	fb = make([]byte, curSym.Func.PCFile.Size)
	fs.ReadAt(fb, curSym.Func.PCFile.Offset)
	module.pclntable = append(module.pclntable, pcFileHead[:pcFileHeadSize-1]...)
	module.pclntable = append(module.pclntable, fb...)

	pclnOff := len(module.pclntable)
	fb = make([]byte, curSym.Func.PCLine.Size)
	fs.ReadAt(fb, curSym.Func.PCLine.Offset)
	module.pclntable = append(module.pclntable, fb...)

	var fInfo funcInfoData
	fInfo._func = init_func(curSym, reloc.SymMap[symName], nameOff, spOff, pcfileOff, pclnOff)
	for _, data := range curSym.Func.PCData {
		fInfo.pcdata = append(fInfo.pcdata, uint32(len(module.pclntable)))
		var b = make([]byte, data.Size)
		fs.ReadAt(b, data.Offset)
		module.pclntable = append(module.pclntable, b...)
	}
	for _, data := range curSym.Func.FuncData {
		var offset uintptr
		if off, ok := reloc.GCObjs[data.Sym.Name]; !ok {
			if gcobj, ok := objsymmap[data.Sym.Name]; ok {
				var b = make([]byte, gcobj.sym.Data.Size)
				cfs := readAtSeeker{ReadSeeker: gcobj.file}
				cfs.ReadAt(b, gcobj.sym.Data.Offset)
				offset = uintptr(len(module.stkmaps))
				module.stkmaps = append(module.stkmaps, b)
				reloc.GCObjs[data.Sym.Name] = offset
			} else if len(data.Sym.Name) == 0 {
				offset = 0xFFFFFFFF
			} else {
				fmt.Println("unknown gcobj:", data.Sym.Name)
			}
		} else {
			offset = off
		}
		if strings.Contains(data.Sym.Name, "stkobj") {
			fInfo.stkobjReloc = objsymmap[data.Sym.Name].sym.Reloc
		}
		fInfo.funcdata = append(fInfo.funcdata, offset)
	}
	fInfo.Var = curSym.Func.Var
	fInfo.name = curSym.Name

	module.ftab = append(module.ftab, functab{
		entry: uintptr(reloc.SymMap[symName]),
	})

	module.funcinfo = append(module.funcinfo, fInfo)
}

func addModule(codeModule *CodeModule, aModule *moduledata) {
	modules[aModule] = true
	for datap := &firstmoduledata; ; {
		if datap.next == nil {
			datap.next = aModule
			break
		}
		datap = datap.next
	}
	codeModule.Module = aModule
}

func removeModule(module interface{}) {
	prevp := &firstmoduledata
	for datap := &firstmoduledata; datap != nil; {
		if datap == module {
			prevp.next = datap.next
			break
		}
		prevp = datap
		datap = datap.next
	}
	delete(modules, module)
}
