package goloader

import (
	"bytes"
	"cmd/objfile/goobj"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

func mustOK(err error) {
	if err != nil {
		panic(err)
	}
}

// copy from $GOROOT/src/cmd/internal/objabi/reloctype.go
const (
	// R_TLS_LE, used on 386, amd64, and ARM, resolves to the offset of the
	// thread-local symbol from the thread local base and is used to implement the
	// "local exec" model for tls access (r.Sym is not set on intel platforms but is
	// set to a TLS symbol -- runtime.tlsg -- in the linker when externally linking).
	R_TLS_LE    = 16
	R_CALL      = 8
	R_CALLARM   = 9
	R_CALLARM64 = 10
	R_CALLIND   = 11
	R_PCREL     = 15
	R_ADDR      = 1
	// R_ADDRARM64 relocates an adrp, add pair to compute the address of the
	// referenced symbol.
	R_ADDRARM64 = 3
	// R_ADDROFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	R_ADDROFF = 5
	// R_WEAKADDROFF resolves just like R_ADDROFF but is a weak relocation.
	// A weak relocation does not make the symbol it refers to reachable,
	// and is only honored by the linker if the symbol is in some other way
	// reachable.
	R_WEAKADDROFF = 6
	// R_METHODOFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	// It is a variant of R_ADDROFF used when linking from the uncommonType of a
	// *rtype, and may be set to zero by the linker if it determines the method
	// text is unreachable by the linked program.
	R_METHODOFF = 24
)

// copy from $GOROOT/src/cmd/internal/objabi/symkind.go
const (
	// An otherwise invalid zero value for the type
	Sxxx = iota
	// Executable instructions
	STEXT
	// Read only static data
	SRODATA
	// Static data that does not contain any pointers
	SNOPTRDATA
	// Static data
	SDATA
	// Statically data that is initially all 0s
	SBSS
	// Statically data that is initially all 0s and does not contain pointers
	SNOPTRBSS
	// Thread-local data that is initally all 0s
	STLSBSS
	// Debugging data
	SDWARFINFO
	SDWARFRANGE
)

type SymData struct {
	Name   string
	Kind   int
	Offset int
	Reloc  []Reloc
}

type Reloc struct {
	Offset int
	SymOff int
	Size   int
	Type   int
	Add    int
}

// CodeReloc dispatch and load CodeReloc struct via network is OK
type CodeReloc struct {
	Code []byte
	Data []byte
	Mod  Module
	Syms []SymData
}

type CodeModule struct {
	Syms       map[string]uintptr
	CodeByte   []byte
	Module     interface{}
	pcfuncdata []findfuncbucket
	stkmaps    [][]byte
	itabs      []itabReloc
	itabSyms   []itabSym
	typemap    map[typeOff]uintptr
}

type itabSym struct {
	ptr   int
	inter int
	_type int
}

type itabReloc struct {
	locOff  int
	symOff  int
	size    int
	locType int
	add     int
}

type symFile struct {
	sym  *goobj.Sym
	file *os.File
}

var (
	tmpModule   interface{}
	modules     = make(map[interface{}]bool)
	modulesLock sync.Mutex
	mov32bit    = [8]byte{0x00, 0x00, 0x80, 0xD2, 0x00, 0x00, 0xA0, 0xF2}
)

func ReadObj(f *os.File) (*CodeReloc, error) {
	obj, err := goobj.Parse(f, "main")
	if err != nil {
		return nil, fmt.Errorf("read error: %v", err)
	}

	var syms = make(map[string]symFile)
	for _, sym := range obj.Syms {
		syms[sym.Name] = symFile{
			sym:  sym,
			file: f,
		}
	}

	var symMap = make(map[string]int)
	var gcObjs = make(map[string]uintptr)
	var fileTabOffsetMap = make(map[string]int)

	var reloc CodeReloc

	for _, sym := range obj.Syms {
		if sym.Kind == STEXT && sym.DupOK == false {
			relocSym(&reloc, symFile{sym: sym,
				file: f}, syms, symMap,
				gcObjs, fileTabOffsetMap)
		} else if sym.Kind == SRODATA {
			if strings.HasPrefix(sym.Name, "type.") {
				relocSym(&reloc, symFile{sym: sym,
					file: f}, syms, symMap,
					gcObjs, fileTabOffsetMap)
			}
		}
	}

	return &reloc, nil
}

func ReadObjs(files []string, pkgPath []string) (*CodeReloc, error) {

	var fs []*os.File
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		fs = append(fs, f)
		defer f.Close()
	}

	var allSyms = make(map[string]symFile)

	var symMap = make(map[string]int)
	var gcObjs = make(map[string]uintptr)
	var fileTabOffsetMap = make(map[string]int)

	var reloc CodeReloc

	var goObjs []*goobj.Package
	for i, f := range fs {
		if pkgPath[i] == "" {
			pkgPath[i] = "main"
		}
		obj, err := goobj.Parse(f, pkgPath[i])
		if err != nil {
			return nil, fmt.Errorf("read error: %v", err)
		}

		for _, sym := range obj.Syms {
			allSyms[sym.Name] = symFile{
				sym:  sym,
				file: f,
			}
		}
		goObjs = append(goObjs, obj)
	}

	for i, obj := range goObjs {
		for _, sym := range obj.Syms {
			if sym.Kind == STEXT && sym.DupOK == false {
				relocSym(&reloc, symFile{sym: sym,
					file: fs[i]}, allSyms, symMap,
					gcObjs, fileTabOffsetMap)
			} else if sym.Kind == SRODATA {
				if strings.HasPrefix(sym.Name, "type.") {
					relocSym(&reloc, symFile{sym: sym,
						file: fs[i]}, allSyms, symMap,
						gcObjs, fileTabOffsetMap)
				}
			}
		}
	}

	return &reloc, nil
}

func addSym(symMap map[string]int, symArray *[]SymData, rsym *SymData) int {
	var offset int
	if of, ok := symMap[rsym.Name]; !ok {
		offset = len(*symArray)
		*symArray = append(*symArray, *rsym)
		symMap[rsym.Name] = offset
	} else {
		offset = of
		(*symArray)[offset] = *rsym
	}
	return offset
}

type readAtSeeker struct {
	io.ReadSeeker
}

func (r *readAtSeeker) ReadAt(p []byte, offset int64) (n int, err error) {
	_, err = r.Seek(offset, io.SeekStart)
	if err != nil {
		return
	}
	return r.Read(p)
}

func relocSym(reloc *CodeReloc, curSym symFile,
	allSyms map[string]symFile, symMap map[string]int,
	gcObjs map[string]uintptr, fileTabOffsetMap map[string]int) int {

	if curSymOffset, ok := symMap[curSym.sym.Name]; ok {
		return curSymOffset
	}

	var rsym SymData
	rsym.Name = curSym.sym.Name
	rsym.Kind = int(curSym.sym.Kind)
	curSymOffset := addSym(symMap, &reloc.Syms, &rsym)

	code := make([]byte, curSym.sym.Data.Size)
	curSym.file.Seek(curSym.sym.Data.Offset, io.SeekStart)
	_, err := curSym.file.Read(code)
	mustOK(err)
	switch int(curSym.sym.Kind) {
	case STEXT:
		rsym.Offset = len(reloc.Code)
		reloc.Code = append(reloc.Code, code...)
		readFuncData(&reloc.Mod, curSym, allSyms, gcObjs,
			fileTabOffsetMap, curSymOffset, rsym.Offset)
	default:
		rsym.Offset = len(reloc.Data)
		reloc.Data = append(reloc.Data, code...)
	}
	addSym(symMap, &reloc.Syms, &rsym)

	for _, re := range curSym.sym.Reloc {
		symOff := -1
		if s, ok := allSyms[re.Sym.Name]; ok {
			symOff = relocSym(reloc, s, allSyms, symMap,
				gcObjs, fileTabOffsetMap)
		} else {
			var exSym SymData
			exSym.Name = re.Sym.Name
			exSym.Offset = -1
			if re.Type == R_TLS_LE {
				exSym.Name = TLSNAME
				exSym.Offset = int(re.Offset)
			}
			if re.Type == R_CALLIND {
				exSym.Offset = 0
				exSym.Name = R_CALLIND_NAME
			}
			if strings.HasPrefix(exSym.Name, "type..importpath.") {
				path := strings.TrimLeft(exSym.Name, "type..importpath.")
				path = strings.Trim(path, ".")
				pathb := []byte(path)
				pathb = append(pathb, 0)
				exSym.Offset = len(reloc.Data)
				reloc.Data = append(reloc.Data, pathb...)
			}
			symOff = addSym(symMap, &reloc.Syms, &exSym)
		}
		rsym.Reloc = append(rsym.Reloc,
			Reloc{Offset: int(re.Offset) + rsym.Offset, SymOff: symOff,
				Type: int(re.Type),
				Size: int(re.Size), Add: int(re.Add)})
	}
	reloc.Syms[curSymOffset].Reloc = rsym.Reloc

	return curSymOffset
}

func strWrite(buf *bytes.Buffer, str ...string) {
	for _, s := range str {
		buf.WriteString(s)
		if s != "\n" {
			buf.WriteString(" ")
		}
	}
}

func relocADRP(mCode []byte, pc int, symAddr int, symName string) {
	pcPage := pc - pc&0xfff
	lowOff := symAddr & 0xfff
	symPage := symAddr - lowOff
	pageOff := symPage - pcPage
	if pageOff > 0x7FFFFFFF || pageOff < -0x80000000 {
		// fmt.Println("adrp overflow!", symName, symAddr, symAddr < (1<<31))
		movlow := binary.LittleEndian.Uint32(mov32bit[:4])
		movhigh := binary.LittleEndian.Uint32(mov32bit[4:])
		adrp := binary.LittleEndian.Uint32(mCode)
		symAddrUint32 := uint32(symAddr)
		movlow = (((adrp & 0x1f) | movlow) | ((symAddrUint32 & 0xffff) << 5))
		movhigh = (((adrp & 0x1f) | movhigh) | ((symAddrUint32 & 0xffff0000) >> 16 << 5))
		// fmt.Println(adrp, movlow, movhigh)
		binary.LittleEndian.PutUint32(mCode, movlow)
		binary.LittleEndian.PutUint32(mCode[4:], movhigh)
		return
	}
	fmt.Println("pageOff<0:", pageOff < 0)
	// 2bit + 19bit + low(12bit) = 33bit
	pageAnd := (uint32((pageOff>>12)&3) << 29) | (uint32((pageOff>>15)&0x7ffff) << 5)

	adrp := binary.LittleEndian.Uint32(mCode)
	adrp = adrp | pageAnd
	binary.LittleEndian.PutUint32(mCode, adrp)

	lowOff = lowOff << 10
	adrpAdd := binary.LittleEndian.Uint32(mCode[4:])
	adrpAdd = adrpAdd | uint32(lowOff)
	binary.LittleEndian.PutUint32(mCode[4:], adrpAdd)
}

func copy2Slice(dst []byte, src unsafe.Pointer, size int) {
	var s = sliceHeader{
		Data: (uintptr)(src),
		Len:  size,
		Cap:  size,
	}
	copy(dst, *(*[]byte)(unsafe.Pointer(&s)))
}

func (cm *CodeModule) Unload() {
	runtime.GC()
	modulesLock.Lock()
	removeModule(cm.Module)
	modulesLock.Unlock()
	Munmap(cm.CodeByte)
}
