package goloader

import (
	"bytes"
	"cmd/objfile/goobj"
	"encoding/binary"
	"errors"
	"fmt"
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

type SymData struct {
	Name   string
	Kind   int
	Offset int
	Reloc  []Reloc
}

type Reloc struct {
	Offset   int
	SymOff   int
	Size     int
	Type     int
	Add      int
	DataSize int64
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
	itabs      map[string]*itabSym
}

type itabSym struct {
	Reloc
	inter *interfacetype
	typ   *_type
	ptr   *itab
}

type objSym struct {
	sym  *goobj.Sym
	file *os.File
}

type segment struct {
	codeBase  int
	dataBase  int
	dataLen   int
	codeLen   int
	maxLength int
	offset    int
	symAddrs  []uintptr
	codeByte  []byte
	err       bytes.Buffer
}

var (
	modules          = make(map[interface{}]bool)
	modulesLock      sync.Mutex
	movcode          byte = 0x8b
	leacode          byte = 0x8d
	cmplcode         byte = 0x83
	x86moduleHead         = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x1, PtrSize}
	armmoduleHead         = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x4, PtrSize}
	armcode               = []byte{0x04, 0xF0, 0x1F, 0xE5}
	arm64code             = []byte{0x49, 0x00, 0x00, 0x58, 0x20, 0x01, 0x1F, 0xD6}
	x86amd64JMPLcode      = []byte{0xff, 0x25, 0x00, 0x00, 0x00, 0x00} // JMPL *ADDRESS

	x86amd64replaceCMPLcode = []byte{
		0x50,                                     // PUSH EAX
		0x53,                                     // PUSH EBX
		0x48, 0x8b, 0x05, 0x0f, 0x00, 0x00, 0x00, // MOVE EAX x
		0x48, 0x8b, 0x18, // MOVE EBX [EAX]
		0x48, 0x83, 0xfb, 0x00, // CMPL EBX x(8bits)
		0x5b,                               // POP EBX
		0x58,                               // POP EAX
		0xff, 0x25, 0x08, 0x00, 0x00, 0x00} // JMPL *ADDRESS

	x86amd64replaceMOVQcode = []byte{
		0x48, 0xb8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //MOVE RxX x
		0xff, 0x25, 0x00, 0x00, 0x00, 0x00} //JMPL *ADDRESS
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

	for _, loc := range objsym.sym.Reloc {
		symOff := INVALID_OFFSET
		if s, ok := objsymmap[loc.Sym.Name]; ok {
			symOff = relocSym(reloc, s.sym.Name, objsymmap)
		} else {
			var exsym SymData
			exsym.Name = loc.Sym.Name
			exsym.Offset = INVALID_OFFSET
			if loc.Type == R_TLS_LE {
				exsym.Name = TLSNAME
				exsym.Offset = int(loc.Offset)
			}
			if loc.Type == R_CALLIND {
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
		rsym.Reloc = append(rsym.Reloc, Reloc{Offset: int(loc.Offset) + rsym.Offset, SymOff: symOff, Type: int(loc.Type), Size: int(loc.Size), Add: int(loc.Add), DataSize: -1})
		if s, ok := objsymmap[loc.Sym.Name]; ok {
			if s.sym.Data.Size == 0 && loc.Size > 0 {
				rsym.Reloc[len(rsym.Reloc)-1].DataSize = s.sym.Data.Size
			}
		}
	}
	reloc.Syms[reloc.SymMap[symName]].Reloc = rsym.Reloc

	return reloc.SymMap[symName]
}

func relocateADRP(mCode []byte, loc Reloc, seg *segment, symAddr uintptr, symName string) {
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
		if sym.Offset == INVALID_OFFSET {
			if ptr, ok := symPtr[sym.Name]; ok {
				seg.symAddrs[i] = ptr
			} else {
				seg.symAddrs[i] = _INVALID_HANDLE_VALUE
				sprintf(&seg.err, "unresolve external:", sym.Name, "\n")
			}
		} else if sym.Name == TLSNAME {
			RegTLS(symPtr, sym.Offset)
		} else if sym.Kind == STEXT {
			seg.symAddrs[i] = uintptr(code.Syms[i].Offset + seg.codeBase)
			codeModule.Syms[sym.Name] = uintptr(seg.symAddrs[i])
		} else if strings.HasPrefix(sym.Name, "go.itab") {
			if ptr, ok := symPtr[sym.Name]; ok {
				seg.symAddrs[i] = ptr
			}
		} else {
			seg.symAddrs[i] = uintptr(code.Syms[i].Offset + seg.dataBase)
			if strings.HasPrefix(sym.Name, "type.") {
				if ptr, ok := symPtr[sym.Name]; ok {
					seg.symAddrs[i] = ptr
				}
			}
		}
	}
}

func relocateItab(code *CodeReloc, module *CodeModule, seg *segment) {
	for itabName, iter := range module.itabs {
		curSym := code.Syms[code.SymMap[itabName]]
		inter := seg.symAddrs[curSym.Reloc[0].SymOff]
		typ := seg.symAddrs[curSym.Reloc[1].SymOff]
		if inter != _INVALID_HANDLE_VALUE && typ != _INVALID_HANDLE_VALUE {
			iter.inter = (*interfacetype)(unsafe.Pointer(inter))
			iter.typ = (*_type)(unsafe.Pointer(typ))
			x := iter.typ.uncommon()
			mhdr := iter.inter.mhdr
			xmhdr := (*[1 << 16]method)(add(unsafe.Pointer(x), uintptr(x.moff)))[:len(mhdr)]
			for k := 0; k < len(iter.inter.mhdr); k++ {
				xmhdr[k].mtyp = typeOff((uintptr)((unsafe.Pointer)(iter.inter.typ.typeOff(mhdr[k].ityp))) - (uintptr)(seg.codeBase))
			}
			iter.ptr = getitab(iter.inter, iter.typ, false)
			module.Module.(*moduledata).itablinks = append(module.Module.(*moduledata).itablinks, iter.ptr)
			address := uintptr(unsafe.Pointer(iter.ptr))
			if iter.ptr != nil {
				switch iter.Type {
				case R_PCREL:
					offset := int(address) - (seg.codeBase + iter.Offset + iter.Size) + iter.Add
					if offset > 0x7FFFFFFF || offset < -0x80000000 {
						offset = (seg.codeBase + seg.offset) - (seg.codeBase + iter.Offset + iter.Size) + iter.Add
						binary.LittleEndian.PutUint32(seg.codeByte[iter.Offset:], uint32(offset))
						if seg.codeByte[iter.Offset-2] == movcode {
							//!!!TRICK
							//because struct itab doesn't change after it adds into itab list, so
							//copy itab data instead of jump code
							copy2Slice(seg.codeByte[seg.offset:], unsafe.Pointer(iter.ptr), itabSize)
							seg.offset += itabSize
						} else if seg.codeByte[iter.Offset-2] == leacode {
							seg.codeByte[iter.Offset-2:][0] = movcode
							*(*uintptr)(unsafe.Pointer(&(seg.codeByte[seg.offset:][0]))) = uintptr(unsafe.Pointer(iter.ptr))
							seg.offset += PtrSize
						} else {
							sprintf(&seg.err, "relocateItab: not support code!", fmt.Sprintf("%v", seg.codeByte[iter.Offset-2:iter.Offset]), "\n")
						}

					} else {
						binary.LittleEndian.PutUint32(seg.codeByte[iter.Offset:], uint32(offset))
					}
				case R_ADDRARM64:
					relocateADRP(seg.codeByte[iter.Offset:], iter.Reloc, seg, address, itabName)
				case R_ADDR:
					*(*uintptr)(unsafe.Pointer(&(seg.codeByte[iter.Offset:][0]))) = uintptr(int(address) + iter.Add)
				default:
					sprintf(&seg.err, "unknown relocateItab type:", strconv.Itoa(iter.Type), "Name:", itabName, "\n")
				}
			}
		}
	}
}

func relocate(code *CodeReloc, symPtr map[string]uintptr, codeModule *CodeModule, seg *segment) {
	for _, curSym := range code.Syms {
		for _, loc := range curSym.Reloc {
			addr := seg.symAddrs[loc.SymOff]
			sym := code.Syms[loc.SymOff]
			//static_tmp is 0, golang compile not allocate memory.
			if loc.DataSize == 0 && loc.Size > 0 {
				if loc.Size <= IntSize {
					addr = uintptr(seg.codeBase + seg.codeLen + seg.dataLen)
				} else {
					sprintf(&seg.err, "Symbol:", sym.Name, "size:", strconv.Itoa(loc.Size), ">IntSize:", strconv.Itoa(IntSize), "\n")
				}
			}
			if addr == _INVALID_HANDLE_VALUE {
				//nothing todo
			} else if addr == 0 && strings.HasPrefix(sym.Name, "go.itab") {
				codeModule.itabs[sym.Name] = &itabSym{Reloc: loc, inter: nil, typ: nil, ptr: nil}
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
					offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add

					if offset > 0x7FFFFFFF || offset < -0x80000000 {
						if seg.offset+PtrSize > seg.maxLength {
							sprintf(&seg.err, "len overflow! sym:", sym.Name, "\n")
						} else {
							offset = (seg.codeBase + seg.offset) - (addrBase + loc.Offset + loc.Size)
							bytes := relocByte[loc.Offset-2:]
							address := addr
							opcode := relocByte[loc.Offset-2]
							reginfo := byte(0x00)
							if loc.Type == R_CALL {
								address = uintptr(int(addr) + loc.Add)
								copy(seg.codeByte[seg.offset:], x86amd64JMPLcode)
								seg.offset += len(x86amd64JMPLcode)
							} else if opcode == leacode {
								bytes[0] = movcode
							} else if opcode == movcode && loc.Size >= Uint32Size {
								reginfo = ((relocByte[loc.Offset-1] >> 3) & 0x7) | 0xb8
								copy(bytes, x86amd64JMPLcode)
							} else if opcode == cmplcode && loc.Size >= Uint32Size {
								copy(bytes, x86amd64JMPLcode)
							} else {
								sprintf(&seg.err, "not support code!", fmt.Sprintf("%v", relocByte[loc.Offset-2:loc.Offset]), "\n")
							}
							binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
							if opcode == cmplcode {
								putAddress(seg.codeByte[seg.offset:], uint64(seg.codeBase+seg.offset+PtrSize))
								seg.offset += PtrSize
								copy(seg.codeByte[seg.offset:], x86amd64replaceCMPLcode)
								seg.codeByte[seg.offset+0xf] = relocByte[loc.Offset+loc.Size]
								seg.offset += len(x86amd64replaceCMPLcode)
								putAddress(seg.codeByte[seg.offset:], uint64(address))
								seg.offset += PtrSize
								address = uintptr(addrBase + loc.Offset + loc.Size - loc.Add)
								putAddress(seg.codeByte[seg.offset:], uint64(address))
								seg.offset += PtrSize
							} else if opcode == movcode {
								putAddress(seg.codeByte[seg.offset:], uint64(seg.codeBase+seg.offset+PtrSize))
								seg.offset += PtrSize
								copy(seg.codeByte[seg.offset:], x86amd64replaceMOVQcode)
								seg.codeByte[seg.offset+1] = reginfo
								address = *(*uintptr)(unsafe.Pointer(address))
								putAddress(seg.codeByte[seg.offset+2:], uint64(address))
								seg.offset += len(x86amd64replaceMOVQcode)
								address = uintptr(addrBase + loc.Offset + loc.Size - loc.Add)
								putAddress(seg.codeByte[seg.offset:], uint64(address))
								seg.offset += PtrSize
							} else {
								putAddress(seg.codeByte[seg.offset:], uint64(address))
								seg.offset += PtrSize
							}

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
					offset := (int(addr) + add - (seg.codeBase + loc.Offset)) / 4
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
							*(*uintptr)(unsafe.Pointer(&(seg.codeByte[seg.offset:][0]))) = uintptr(int(addr) + add)
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
					relocateADRP(seg.codeByte[loc.Offset:], loc, seg, addr, sym.Name)
				case R_ADDR:
					var relocByte = seg.codeByte[seg.codeLen:]
					if curSym.Kind == STEXT {
						relocByte = seg.codeByte
					}
					address := uintptr(int(addr) + loc.Add)
					*(*uintptr)(unsafe.Pointer(&(relocByte[loc.Offset:][0]))) = uintptr(address)
				case R_CALLIND:

				case R_ADDROFF, R_WEAKADDROFF, R_METHODOFF:
					if curSym.Kind == STEXT {
						sprintf(&seg.err, "impossible!", sym.Name, " locate on code segment!\n")
					}
					offset := int(addr) - seg.codeBase + loc.Add
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
		if code.Mod.stkmaps[v] != nil {
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
	module.typemap = nil
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
		Syms:  make(map[string]uintptr),
		itabs: make(map[string]*itabSym),
	}

	seg.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	seg.dataBase = seg.codeBase + len(code.Code)
	seg.symAddrs = make([]uintptr, len(code.Syms))
	seg.offset = seg.codeLen + seg.dataLen
	//static_tmp is 0, golang compile not allocate memory.
	seg.offset += IntSize
	copy(seg.codeByte, code.Code)
	copy(seg.codeByte[seg.codeLen:], code.Data)

	addSymAddrs(code, symPtr, &codeModule, &seg)
	relocate(code, symPtr, &codeModule, &seg)
	buildModule(code, symPtr, &codeModule, &seg)
	relocateItab(code, &codeModule, &seg)

	if seg.err.Len() > 0 {
		return &codeModule, errors.New(seg.err.String())
	}
	return &codeModule, nil
}

func (cm *CodeModule) Unload() {
	for _, itab := range cm.itabs {
		if itab.inter != nil && itab.typ != nil {
			eraseiface(itab.inter, itab.typ)
		}
	}
	runtime.GC()
	modulesLock.Lock()
	removeModule(cm.Module)
	modulesLock.Unlock()
	Munmap(cm.CodeByte)
}
