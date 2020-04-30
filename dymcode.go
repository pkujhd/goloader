package goloader

import (
	"bytes"
	"cmd/objfile/goobj"
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// copy from $GOROOT/src/cmd/internal/objabi/reloctype.go
const (
	R_ADDR = 1
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
	R_CALL        = 8
	R_CALLARM     = 9
	R_CALLARM64   = 10
	R_CALLIND     = 11
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
	Code    []byte
	Data    []byte
	Mod     Module
	Syms    []SymData
	SymMap  map[string]int
	GCObjs  map[string]uintptr
	FileMap map[string]int
	Arch    string
}

type CodeModule struct {
	Syms       map[string]uintptr
	CodeByte   []byte
	Module     interface{}
	pcfuncdata []findfuncbucket
	stkmaps    [][]byte
	itabs      []itabSym
	typemap    map[typeOff]uintptr
}

type itabSym struct {
	Reloc
	ptr   int
	inter *interfacetype
	_type *_type
}

type objSym struct {
	sym  *goobj.Sym
	file *os.File
}

type segment struct {
	codeBase   int
	dataBase   int
	dataLen    int
	codeLen    int
	maxLength  int
	offset     int
	symAddrs   []int
	itabMap    map[string]int
	funcType   map[string]*int
	codeByte   []byte
	typeSymPtr map[string]uintptr
	err        bytes.Buffer
}

var (
	modules       = make(map[interface{}]bool)
	modulesLock   sync.Mutex
	movcode       byte = 0x8b
	leacode       byte = 0x8d
	x86moduleHead      = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x1, PtrSize}
	armmoduleHead      = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x4, PtrSize}
	armcode            = []byte{0x04, 0xF0, 0x1F, 0xE5}
	arm64code          = []byte{0x49, 0x00, 0x00, 0x58, 0x20, 0x01, 0x1F, 0xD6}
	x86code            = []byte{0xff, 0x25, 0x00, 0x00, 0x00, 0x00}
)

func addSymMap(symMap map[string]int, symArray *[]SymData, rsym *SymData) int {
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

func relocSym(reloc *CodeReloc, symName string, objsymmap map[string]objSym) int {
	if offset, ok := reloc.SymMap[symName]; ok {
		return offset
	}
	objsym := objsymmap[symName]
	var rsym SymData
	rsym.Name = objsym.sym.Name
	rsym.Kind = int(objsym.sym.Kind)
	addSymMap(reloc.SymMap, &reloc.Syms, &rsym)

	code := make([]byte, objsym.sym.Data.Size)
	_, err := objsym.file.ReadAt(code, objsym.sym.Data.Offset)
	assert(err)
	switch int(objsym.sym.Kind) {
	case STEXT:
		rsym.Offset = len(reloc.Code)
		reloc.Code = append(reloc.Code, code...)
		readFuncData(reloc, symName, objsymmap, rsym.Offset)
	default:
		rsym.Offset = len(reloc.Data)
		reloc.Data = append(reloc.Data, code...)
	}
	addSymMap(reloc.SymMap, &reloc.Syms, &rsym)

	for _, re := range objsym.sym.Reloc {
		symOff := -1
		if s, ok := objsymmap[re.Sym.Name]; ok {
			symOff = relocSym(reloc, s.sym.Name, objsymmap)
		} else {
			var exsym SymData
			exsym.Name = re.Sym.Name
			exsym.Offset = -1
			if re.Type == R_TLS_LE {
				exsym.Name = TLSNAME
				exsym.Offset = int(re.Offset)
			}
			if re.Type == R_CALLIND {
				exsym.Offset = 0
				exsym.Name = R_CALLIND_NAME
			}
			if strings.HasPrefix(exsym.Name, "type..importpath.") {
				path := strings.Trim(strings.TrimLeft(exsym.Name, "type..importpath."), ".")
				pathbytes := []byte(path)
				pathbytes = append(pathbytes, 0)
				exsym.Offset = len(reloc.Data)
				reloc.Data = append(reloc.Data, pathbytes...)
			}
			symOff = addSymMap(reloc.SymMap, &reloc.Syms, &exsym)
		}
		rsym.Reloc = append(rsym.Reloc, Reloc{Offset: int(re.Offset) + rsym.Offset, SymOff: symOff, Type: int(re.Type), Size: int(re.Size), Add: int(re.Add)})
	}
	reloc.Syms[reloc.SymMap[symName]].Reloc = rsym.Reloc

	return reloc.SymMap[symName]
}

func relocateADRP(mCode []byte, loc Reloc, seg *segment, symAddr int, symName string) {
	offset := int64(symAddr) + int64(loc.Add) - ((int64(seg.codeBase) + int64(loc.Offset)) &^ 0xfff)
	//overflow
	if offset > 0xFFFFFFFF || offset <= -0x100000000 {
		//low:	MOV reg imm
		//high: MOVK reg imm LSL#16
		value := uint64(0xF2A00000D2800000)
		addr := binary.LittleEndian.Uint32(mCode)
		low := uint32(value & 0xFFFFFFFF)
		high := uint32(value >> 32)
		low = ((addr & 0x1f) | low) | ((uint32(symAddr) & 0xffff) << 5)
		high = ((addr & 0x1f) | high) | (uint32(symAddr) >> 16 << 5)
		binary.LittleEndian.PutUint64(mCode, uint64(low)|(uint64(high)<<32))
	} else {
		// 2bit + 19bit + low(12bit) = 33bit
		low := (uint32((offset>>12)&3) << 29) | (uint32((offset>>12>>2)&0x7ffff) << 5)
		high := (uint32(offset&0xfff) << 10)
		value := binary.LittleEndian.Uint64(mCode)
		value = (uint64(uint32(value>>32)|high) << 32) | uint64(uint32(value&0xFFFFFFFF)|low)
		binary.LittleEndian.PutUint64(mCode, value)
	}
}

func addSymAddrs(code *CodeReloc, symPtr map[string]uintptr, codeModule *CodeModule, seg *segment) {
	for i, sym := range code.Syms {
		if sym.Offset == -1 {
			if ptr, ok := symPtr[sym.Name]; ok {
				seg.symAddrs[i] = int(ptr)
			} else {
				seg.symAddrs[i] = -1
				sprintf(&seg.err, "unresolve external:", sym.Name, "\n")
			}
		} else if sym.Name == TLSNAME {
			RegTLS(symPtr, sym.Offset)
		} else if sym.Kind == STEXT {
			seg.symAddrs[i] = code.Syms[i].Offset + seg.codeBase
			codeModule.Syms[sym.Name] = uintptr(seg.symAddrs[i])
		} else if strings.HasPrefix(sym.Name, "go.itab") {
			if ptr, ok := symPtr[sym.Name]; ok {
				seg.symAddrs[i] = int(ptr)
			} else {
				seg.itabMap[sym.Name] = i
			}
		} else {
			seg.symAddrs[i] = code.Syms[i].Offset + seg.dataBase

			if strings.HasPrefix(sym.Name, "type.func") {
				seg.funcType[sym.Name] = &seg.symAddrs[i]
			}
			if strings.HasPrefix(sym.Name, "type.") {
				if ptr, ok := symPtr[sym.Name]; ok {
					seg.symAddrs[i] = int(ptr)
				} else {
					seg.typeSymPtr[sym.Name] = (uintptr)(seg.symAddrs[i])
				}
			}
		}
	}
}

func addItab(code *CodeReloc, codeModule *CodeModule, seg *segment) {
	for itabName, itabIndex := range seg.itabMap {
		curSym := code.Syms[itabIndex]
		inter := seg.symAddrs[curSym.Reloc[0].SymOff]
		typ := seg.symAddrs[curSym.Reloc[1].SymOff]
		if inter == -1 || typ == -1 {
			seg.itabMap[itabName] = -1
			continue
		}
		seg.itabMap[itabName] = len(codeModule.itabs)
		codeModule.itabs = append(codeModule.itabs, itabSym{inter: (*interfacetype)((unsafe.Pointer)(uintptr(inter))), _type: (*_type)((unsafe.Pointer)(uintptr(typ)))})
	}
}

func relocateItab(code *CodeReloc, codeModule *CodeModule, seg *segment) {
	for i := range codeModule.itabs {
		it := &codeModule.itabs[i]
		addIFaceSubFuncType(seg.funcType, codeModule.typemap, it.inter, it._type, seg.codeBase)
		it.ptr = getitab(it.inter, it._type, false)
		if it.ptr != 0 {
			switch it.Type {
			case R_PCREL:
				pc := seg.codeBase + it.Offset + it.Size
				offset := it.ptr - pc + it.Add
				if offset > 0x7FFFFFFF || offset < -0x80000000 {
					offset = (seg.codeBase + seg.offset) - pc + it.Add
					binary.LittleEndian.PutUint32(seg.codeByte[it.Offset:], uint32(offset))
					seg.codeByte[it.Offset-2:][0] = movcode
					*(*uintptr)(unsafe.Pointer(&(seg.codeByte[seg.offset:][0]))) = uintptr(it.ptr)
					seg.offset += PtrSize
				} else {
					binary.LittleEndian.PutUint32(seg.codeByte[it.Offset:], uint32(offset))
				}
			case R_ADDRARM64:
				relocateADRP(seg.codeByte[it.Offset:], it.Reloc, seg, it.ptr, it.inter.typ.Name())
			case R_ADDR:
				offset := it.ptr + it.Add
				*(*uintptr)(unsafe.Pointer(&(seg.codeByte[it.Offset:][0]))) = uintptr(offset)
			default:
				sprintf(&seg.err, "unknown relocateItab type:", strconv.Itoa(it.Type), "Name:", it.inter.typ.Name(), "\n")
			}
		}
	}
}

func relocate(code *CodeReloc, symPtr map[string]uintptr, codeModule *CodeModule, seg *segment) {
	for _, curSym := range code.Syms {
		for _, loc := range curSym.Reloc {
			sym := code.Syms[loc.SymOff]
			if seg.symAddrs[loc.SymOff] == -1 {
				//nothing todo
			} else if seg.symAddrs[loc.SymOff] == 0 && strings.HasPrefix(sym.Name, "go.itab") {
				codeModule.itabs[seg.itabMap[sym.Name]].Reloc = loc
			} else {
				switch loc.Type {
				case R_TLS_LE:
					binary.LittleEndian.PutUint32(seg.codeByte[loc.Offset:], uint32(symPtr[TLSNAME]))
				case R_CALL, R_PCREL:
					var relocByte = seg.codeByte[seg.codeLen:]
					var addrBase = seg.dataBase
					if curSym.Kind == STEXT {
						addrBase = seg.codeBase
						relocByte = seg.codeByte
					}
					offset := seg.symAddrs[loc.SymOff] - (addrBase + loc.Offset + loc.Size) + loc.Add
					if offset > 0x7FFFFFFF || offset < -0x8000000 {
						if seg.offset+8 > seg.maxLength {
							sprintf(&seg.err, "len overflow! sym:", sym.Name, "\n")
						} else {
							offset = (seg.codeBase + seg.offset) - (addrBase + loc.Offset + loc.Size)
							rb := relocByte[loc.Offset-2:]
							if loc.Type == R_CALL {
								copy(seg.codeByte[seg.offset:], x86code)
								seg.offset += len(x86code)
							} else if rb[0] == leacode {
								rb[0] = movcode
							}
							binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
							if uint64(seg.symAddrs[loc.SymOff]+loc.Add) > 0xFFFFFFFF {
								binary.LittleEndian.PutUint64(seg.codeByte[seg.offset:], uint64(seg.symAddrs[loc.SymOff]+loc.Add))
							} else {
								binary.LittleEndian.PutUint32(seg.codeByte[seg.offset:], uint32(seg.symAddrs[loc.SymOff]+loc.Add))
							}
							seg.offset += PtrSize
						}
					} else {
						binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
					}
				case R_CALLARM, R_CALLARM64:
					var add = loc.Add
					var pcOff = 0
					if loc.Type == R_CALLARM {
						add = int(signext24(int64(loc.Add&0xFFFFFF)) * 4)
						pcOff = 8
					}
					offset := (seg.symAddrs[loc.SymOff] + add - (seg.codeBase + loc.Offset)) / 4
					if offset > 0x7FFFFF || offset < -0x800000 {
						if seg.offset+PtrSize > seg.maxLength {
							sprintf(&seg.err, "len overflow! sym:", sym.Name, "\n")
						} else {
							seg.offset += (PtrSize - seg.offset%PtrSize)
							if loc.Type == R_CALLARM {
								add = int(signext24(int64(loc.Add&0xFFFFFF)+2) * 4)
							}
							putUint24(seg.codeByte[loc.Offset:], uint32(seg.offset-pcOff-loc.Offset)/4)
							if loc.Type == R_CALLARM64 {
								copy(seg.codeByte[seg.offset:], arm64code)
								seg.offset += len(arm64code)
							} else {
								copy(seg.codeByte[seg.offset:], armcode)
								seg.offset += len(armcode)
							}
							*(*uintptr)(unsafe.Pointer(&(seg.codeByte[seg.offset:][0]))) = uintptr(seg.symAddrs[loc.SymOff] + add)
							seg.offset += PtrSize
						}
					} else {
						val := binary.LittleEndian.Uint32(seg.codeByte[loc.Offset : loc.Offset+4])
						if loc.Type == R_CALLARM {
							val |= uint32(offset) & 0x00FFFFFF
						} else {
							val |= uint32(offset) & 0x03FFFFFF
						}
						binary.LittleEndian.PutUint32(seg.codeByte[loc.Offset:], val)
					}
				case R_ADDRARM64:
					if curSym.Kind != STEXT {
						sprintf(&seg.err, "impossible!", sym.Name, "locate not in code segment!\n")
					}
					relocateADRP(seg.codeByte[loc.Offset:], loc, seg, seg.symAddrs[loc.SymOff], sym.Name)
				case R_ADDR:
					var relocByte = seg.codeByte[seg.codeLen:]
					if curSym.Kind == STEXT {
						relocByte = seg.codeByte
					}
					offset := seg.symAddrs[loc.SymOff] + loc.Add
					*(*uintptr)(unsafe.Pointer(&(relocByte[loc.Offset:][0]))) = uintptr(offset)
				case R_CALLIND:

				case R_ADDROFF, R_WEAKADDROFF, R_METHODOFF:
					if curSym.Kind == STEXT {
						sprintf(&seg.err, "impossible!", sym.Name, " locate on code segment!\n")
					}
					offset := seg.symAddrs[loc.SymOff] - seg.codeBase + loc.Add
					binary.LittleEndian.PutUint32(seg.codeByte[seg.codeLen+loc.Offset:], uint32(offset))
				default:
					sprintf(&seg.err, "unknown reloc type:", strconv.Itoa(loc.Type), " sym:", sym.Name, "\n")
				}
			}

		}
	}
}

func addFuncTab(module *moduledata, i, pclnOff int, code *CodeReloc, seg *segment, symPtr map[string]uintptr) int {
	module.ftab[i].entry = uintptr(seg.symAddrs[int(code.Mod.ftab[i].entry)])

	if pclnOff%PtrSize != 0 {
		pclnOff = pclnOff + (PtrSize - pclnOff%PtrSize)
	}
	module.ftab[i].funcoff = uintptr(pclnOff)
	fi := code.Mod.funcinfo[i]
	fi.entry = module.ftab[i].entry

	var funcdata = make([]uintptr, len(fi.funcdata))
	copy(funcdata, fi.funcdata)
	for i, v := range fi.funcdata {
		if v != 0xFFFFFFFF {
			funcdata[i] = (uintptr)(unsafe.Pointer(&(code.Mod.stkmaps[v][0])))
		} else {
			funcdata[i] = (uintptr)(0)
		}
	}

	addStackObject(code, &fi, seg, symPtr)
	addDeferReturn(code, &fi, seg)

	copy2Slice(module.pclntable[pclnOff:], unsafe.Pointer(&fi._func), _funcSize)
	pclnOff += _funcSize

	if len(fi.pcdata) > 0 {
		size := int(int32(unsafe.Sizeof(fi.pcdata[0])) * fi.npcdata)
		copy2Slice(module.pclntable[pclnOff:], unsafe.Pointer(&fi.pcdata[0]), size)
		pclnOff += size
	}

	if pclnOff%PtrSize != 0 {
		pclnOff = pclnOff + (PtrSize - pclnOff%PtrSize)
	}

	funcDataSize := int(PtrSize * fi.nfuncdata)
	copy2Slice(module.pclntable[pclnOff:], unsafe.Pointer(&funcdata[0]), funcDataSize)
	pclnOff += funcDataSize

	return pclnOff
}

func buildModule(code *CodeReloc, symPtr map[string]uintptr, codeModule *CodeModule, seg *segment) {
	var module moduledata
	module.ftab = make([]functab, len(code.Mod.ftab))
	copy(module.ftab, code.Mod.ftab)
	pclnOff := len(code.Mod.pclntable)
	module.pclntable = make([]byte, len(code.Mod.pclntable)+
		(_funcSize+128)*len(code.Mod.ftab))
	copy(module.pclntable, code.Mod.pclntable)
	module.findfunctab = (uintptr)(unsafe.Pointer(&code.Mod.pcfunc[0]))
	module.minpc = uintptr(seg.codeBase)
	module.maxpc = uintptr(seg.dataBase)
	module.filetab = code.Mod.filetab
	module.typemap = codeModule.typemap
	module.types = uintptr(seg.codeBase)
	module.etypes = uintptr(seg.codeBase + seg.codeLen + seg.dataLen)
	module.text = uintptr(seg.codeBase)
	module.etext = uintptr(seg.codeBase + len(code.Code))
	codeModule.pcfuncdata = code.Mod.pcfunc // hold reference
	codeModule.stkmaps = code.Mod.stkmaps
	for i := range module.ftab {
		pclnOff = addFuncTab(&module, i, pclnOff, code, seg, symPtr)
	}
	module.pclntable = module.pclntable[:pclnOff]
	module.ftab = append(module.ftab, functab{})
	for i := len(module.ftab) - 1; i > 0; i-- {
		module.ftab[i] = module.ftab[i-1]
	}
	module.ftab = append(module.ftab, functab{})
	module.ftab[0].entry = module.minpc
	module.ftab[len(module.ftab)-1].entry = module.maxpc

	modulesLock.Lock()
	addModule(codeModule, &module)
	modulesLock.Unlock()
	moduledataverify1(&module)

	codeModule.CodeByte = seg.codeByte
}

func Load(code *CodeReloc, symPtr map[string]uintptr) (*CodeModule, error) {
	var seg segment
	seg.codeLen = len(code.Code)
	seg.dataLen = len(code.Data)
	seg.maxLength = seg.codeLen*2 + seg.dataLen
	codeByte, err := Mmap(seg.maxLength)
	if err != nil {
		return nil, err
	}
	seg.codeByte = codeByte

	var codeModule = CodeModule{
		Syms:    make(map[string]uintptr),
		typemap: make(map[typeOff]uintptr),
	}

	seg.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	seg.dataBase = seg.codeBase + len(code.Code)
	seg.symAddrs = make([]int, len(code.Syms))
	seg.funcType = make(map[string]*int)
	seg.itabMap = make(map[string]int)
	seg.typeSymPtr = make(map[string]uintptr)
	seg.offset = seg.codeLen + seg.dataLen
	copy(seg.codeByte, code.Code)
	copy(seg.codeByte[seg.codeLen:], code.Data)

	addSymAddrs(code, symPtr, &codeModule, &seg)
	addItab(code, &codeModule, &seg)
	relocate(code, symPtr, &codeModule, &seg)
	buildModule(code, symPtr, &codeModule, &seg)
	relocateItab(code, &codeModule, &seg)

	if seg.err.Len() > 0 {
		return &codeModule, errors.New(seg.err.String())
	}
	return &codeModule, nil
}

func (cm *CodeModule) Unload() {
	runtime.GC()
	modulesLock.Lock()
	removeModule(cm.Module)
	modulesLock.Unlock()
	Munmap(cm.CodeByte)
}
