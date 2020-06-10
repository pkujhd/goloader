package goloader

import (
	"cmd/objfile/goobj"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"runtime"
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
	Sym      *SymData
	Size     int
	Type     int
	Add      int
	DataSize int64
}

// CodeReloc dispatch and load CodeReloc struct via network is OK
type CodeReloc struct {
	module
	code    []byte
	data    []byte
	symMap  map[string]*SymData
	stkmaps map[string][]byte
	fileMap map[string]int
	Arch    string
}

type CodeModule struct {
	Syms     map[string]uintptr
	codeByte []byte
	module   *moduledata
	stkmaps  map[string][]byte
	itabs    map[string]*itabSym
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
	codeByte  []byte
	codeBase  int
	dataBase  int
	dataLen   int
	codeLen   int
	maxLength int
	offset    int
	symAddrs  map[string]uintptr
	errors    string
}

var (
	modules       = make(map[interface{}]bool)
	modulesLock   sync.Mutex
	x86moduleHead = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x1, PtrSize}
	armmoduleHead = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x4, PtrSize}
)

func relocSym(codereloc *CodeReloc, name string, objsymmap map[string]objSym) (*SymData, error) {
	if symbol, ok := codereloc.symMap[name]; ok {
		return symbol, nil
	}
	objsym := objsymmap[name]
	rsym := SymData{Name: objsym.sym.Name, Kind: int(objsym.sym.Kind)}
	codereloc.symMap[rsym.Name] = &rsym

	code := make([]byte, objsym.sym.Data.Size)
	_, err := objsym.file.ReadAt(code, objsym.sym.Data.Offset)
	if err != nil {
		return nil, err
	}
	switch rsym.Kind {
	case STEXT:
		rsym.Offset = len(codereloc.code)
		codereloc.code = append(codereloc.code, code...)
		bytearrayAlign(&codereloc.code, PtrSize)
		err := readFuncData(codereloc, name, objsymmap, rsym.Offset)
		if err != nil {
			return nil, err
		}
	default:
		rsym.Offset = len(codereloc.data)
		codereloc.data = append(codereloc.data, code...)
		bytearrayAlign(&codereloc.data, PtrSize)
	}

	for _, loc := range objsym.sym.Reloc {
		symbol := (*SymData)(nil)
		if s, ok := objsymmap[loc.Sym.Name]; ok {
			symbol, err = relocSym(codereloc, s.sym.Name, objsymmap)
			if err != nil {
				return nil, err
			}
		} else {
			sym := SymData{Name: loc.Sym.Name, Offset: INVALID_OFFSET}
			if loc.Type == R_TLS_LE {
				sym.Name = TLSNAME
				sym.Offset = int(loc.Offset)
			}
			if loc.Type == R_CALLIND {
				sym.Offset = 0
				sym.Name = R_CALLIND_NAME
			}
			if strings.HasPrefix(sym.Name, TYPE_IMPORTPATH_PREFIX) {
				path := strings.Trim(strings.TrimLeft(sym.Name, TYPE_IMPORTPATH_PREFIX), ".")
				sym.Offset = len(codereloc.data)
				codereloc.data = append(codereloc.data, path...)
				codereloc.data = append(codereloc.data, ZERO_BYTE)
			}
			codereloc.symMap[sym.Name] = &sym
			symbol = &sym
		}
		rsym.Reloc = append(rsym.Reloc, Reloc{Offset: int(loc.Offset) + rsym.Offset, Sym: symbol, Type: int(loc.Type), Size: int(loc.Size), Add: int(loc.Add), DataSize: -1})
		if s, ok := objsymmap[loc.Sym.Name]; ok {
			if s.sym.Data.Size == 0 && loc.Size > 0 {
				rsym.Reloc[len(rsym.Reloc)-1].DataSize = s.sym.Data.Size
			}
		}
	}
	codereloc.symMap[name].Reloc = rsym.Reloc

	return codereloc.symMap[name], nil
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

func addSymAddrs(codereloc *CodeReloc, symPtr map[string]uintptr, codeModule *CodeModule, seg *segment) {
	for name, sym := range codereloc.symMap {
		if sym.Offset == INVALID_OFFSET {
			if ptr, ok := symPtr[sym.Name]; ok {
				seg.symAddrs[name] = ptr
			} else {
				seg.symAddrs[name] = INVALID_HANDLE_VALUE
				seg.errors += fmt.Sprintf("unresolve external:%s\n", sym.Name)
			}
		} else if sym.Name == TLSNAME {
			regTLS(symPtr, sym.Offset)
		} else if sym.Kind == STEXT {
			seg.symAddrs[name] = uintptr(codereloc.symMap[name].Offset + seg.codeBase)
			codeModule.Syms[sym.Name] = uintptr(seg.symAddrs[name])
		} else if strings.HasPrefix(sym.Name, ITAB_PREFIX) {
			if ptr, ok := symPtr[sym.Name]; ok {
				seg.symAddrs[name] = ptr
			}
		} else {
			seg.symAddrs[name] = uintptr(codereloc.symMap[name].Offset + seg.dataBase)
			if strings.HasPrefix(sym.Name, TYPE_PREFIX) {
				if ptr, ok := symPtr[sym.Name]; ok {
					seg.symAddrs[name] = ptr
				}
			}
		}
	}
}

func relocateItab(codereloc *CodeReloc, codeModule *CodeModule, seg *segment) {
	for itabName, iter := range codeModule.itabs {
		symbol := codereloc.symMap[itabName]
		inter := seg.symAddrs[symbol.Reloc[0].Sym.Name]
		typ := seg.symAddrs[symbol.Reloc[1].Sym.Name]
		if inter != INVALID_HANDLE_VALUE && typ != INVALID_HANDLE_VALUE {
			*(*uintptr)(unsafe.Pointer(&(iter.inter))) = inter
			*(*uintptr)(unsafe.Pointer(&(iter.typ))) = typ
			methods := iter.typ.uncommon().methods()
			for k := 0; k < len(iter.inter.mhdr) && k < len(methods); k++ {
				itype := uintptr(unsafe.Pointer(iter.inter.typ.typeOff(iter.inter.mhdr[k].ityp)))
				codeModule.module.typemap[methods[k].mtyp] = itype
			}
			iter.ptr = getitab(iter.inter, iter.typ, false)
			address := uintptr(unsafe.Pointer(iter.ptr))
			if iter.ptr != nil {
				switch iter.Type {
				case R_PCREL:
					offset := int(address) - (seg.codeBase + iter.Offset + iter.Size) + iter.Add
					if offset > 0x7FFFFFFF || offset < -0x80000000 {
						offset = seg.offset - (iter.Offset + iter.Size) + iter.Add
						binary.LittleEndian.PutUint32(seg.codeByte[iter.Offset:], uint32(offset))
						if seg.codeByte[iter.Offset-2] == x86amd64MOVcode {
							//!!!TRICK
							//because struct itab doesn't change after it adds into itab list, so
							//copy itab data instead of jump code
							copy2Slice(seg.codeByte[seg.offset:], address, ItabSize)
							seg.offset += ItabSize
						} else if seg.codeByte[iter.Offset-2] == x86amd64LEAcode {
							seg.codeByte[iter.Offset-2:][0] = x86amd64MOVcode
							putAddressAddOffset(seg.codeByte, &seg.offset, uint64(address))
						} else {
							seg.errors += fmt.Sprintf("relocateItab: not support code:%v!\n", seg.codeByte[iter.Offset-2:iter.Offset])
						}
					} else {
						binary.LittleEndian.PutUint32(seg.codeByte[iter.Offset:], uint32(offset))
					}
				case R_ADDRARM64:
					relocateADRP(seg.codeByte[iter.Offset:], iter.Reloc, seg, address, itabName)
				case R_ADDR:
					putAddress(seg.codeByte[iter.Offset:], uint64(int(address)+iter.Add))
				default:
					seg.errors += fmt.Sprintf("unknown relocateItab type:%d Name:%s\n", iter.Type, itabName)
				}
			}
		}
	}
}

func relocateCALL(addr uintptr, loc Reloc, seg *segment, sym *SymData, relocByte []byte, addrBase int) {
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		offset = (seg.codeBase + seg.offset) - (addrBase + loc.Offset + loc.Size)
		copy(seg.codeByte[seg.offset:], x86amd64JMPLcode)
		seg.offset += len(x86amd64JMPLcode)
		putAddressAddOffset(seg.codeByte, &seg.offset, uint64(addr)+uint64(loc.Add))
	}
	binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
}

func relocatePCREL(addr uintptr, loc Reloc, seg *segment, sym *SymData, relocByte []byte, addrBase int) {
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		offset = (seg.codeBase + seg.offset) - (addrBase + loc.Offset + loc.Size)
		bytes := relocByte[loc.Offset-2:]
		opcode := relocByte[loc.Offset-2]
		regsiter := ZERO_BYTE
		if opcode == x86amd64LEAcode {
			bytes[0] = x86amd64MOVcode
		} else if opcode == x86amd64MOVcode && loc.Size >= Uint32Size {
			regsiter = ((relocByte[loc.Offset-1] >> 3) & 0x7) | 0xb8
			copy(bytes, x86amd64JMPLcode)
		} else if opcode == x86amd64CMPLcode && loc.Size >= Uint32Size {
			copy(bytes, x86amd64JMPLcode)
		} else {
			seg.errors += fmt.Sprintf("not support code:%v!\n", relocByte[loc.Offset-2:loc.Offset])
		}
		binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
		if opcode == x86amd64CMPLcode || opcode == x86amd64MOVcode {
			putAddressAddOffset(seg.codeByte, &seg.offset, uint64(seg.codeBase+seg.offset+PtrSize))
			if opcode == x86amd64CMPLcode {
				copy(seg.codeByte[seg.offset:], x86amd64replaceCMPLcode)
				seg.codeByte[seg.offset+0x0F] = relocByte[loc.Offset+loc.Size]
				seg.offset += len(x86amd64replaceCMPLcode)
				putAddressAddOffset(seg.codeByte, &seg.offset, uint64(addr))
			} else {
				copy(seg.codeByte[seg.offset:], x86amd64replaceMOVQcode)
				seg.codeByte[seg.offset+1] = regsiter
				copy2Slice(seg.codeByte[seg.offset+2:], addr, PtrSize)
				seg.offset += len(x86amd64replaceMOVQcode)
			}
			putAddressAddOffset(seg.codeByte, &seg.offset, uint64(addrBase+loc.Offset+loc.Size-loc.Add))
		} else {
			putAddressAddOffset(seg.codeByte, &seg.offset, uint64(addr))
		}
	} else {
		binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
	}
}

func relocteCALLARM(addr uintptr, loc Reloc, seg *segment, sym *SymData) {
	add := loc.Add
	if loc.Type == R_CALLARM {
		add = int(signext24(int64(loc.Add&0xFFFFFF)) * 4)
	}
	offset := (int(addr) + add - (seg.codeBase + loc.Offset)) / 4
	if offset > 0x7FFFFF || offset < -0x800000 {
		seg.offset = alignof(seg.offset, PtrSize)
		off := uint32(seg.offset-loc.Offset) / 4
		if loc.Type == R_CALLARM {
			add = int(signext24(int64(loc.Add&0xFFFFFF)+2) * 4)
			off = uint32(seg.offset-loc.Offset-8) / 4
		}
		putUint24(seg.codeByte[loc.Offset:], off)
		if loc.Type == R_CALLARM64 {
			copy(seg.codeByte[seg.offset:], arm64code)
			seg.offset += len(arm64code)
		} else {
			copy(seg.codeByte[seg.offset:], armcode)
			seg.offset += len(armcode)
		}
		putAddressAddOffset(seg.codeByte, &seg.offset, uint64(int(addr)+add))
	} else {
		val := binary.LittleEndian.Uint32(seg.codeByte[loc.Offset:])
		if loc.Type == R_CALLARM {
			val |= uint32(offset) & 0x00FFFFFF
		} else {
			val |= uint32(offset) & 0x03FFFFFF
		}
		binary.LittleEndian.PutUint32(seg.codeByte[loc.Offset:], val)
	}
}

func relocate(codereloc *CodeReloc, symPtr map[string]uintptr, codeModule *CodeModule, seg *segment) {
	for _, symbol := range codereloc.symMap {
		for _, loc := range symbol.Reloc {
			addr := seg.symAddrs[loc.Sym.Name]
			sym := loc.Sym
			relocByte := seg.codeByte[seg.codeLen:]
			addrBase := seg.dataBase
			if symbol.Kind == STEXT {
				addrBase = seg.codeBase
				relocByte = seg.codeByte
			}
			//static_tmp is 0, golang compile not allocate memory.
			if loc.DataSize == 0 && loc.Size > 0 {
				if loc.Size <= IntSize {
					addr = uintptr(seg.codeBase + seg.codeLen + seg.dataLen)
				} else {
					seg.errors += fmt.Sprintf("Symbol:%s size:%d>IntSize:%d\n", sym.Name, loc.Size, IntSize)
				}
			}
			if addr == INVALID_HANDLE_VALUE {
				//nothing todo
			} else if addr == 0 && strings.HasPrefix(sym.Name, ITAB_PREFIX) {
				codeModule.itabs[sym.Name] = &itabSym{Reloc: loc, inter: nil, typ: nil, ptr: nil}
			} else {
				switch loc.Type {
				case R_TLS_LE:
					binary.LittleEndian.PutUint32(seg.codeByte[loc.Offset:], uint32(symPtr[TLSNAME]))
				case R_CALL:
					relocateCALL(addr, loc, seg, sym, relocByte, addrBase)
				case R_PCREL:
					relocatePCREL(addr, loc, seg, sym, relocByte, addrBase)
				case R_CALLARM, R_CALLARM64:
					relocteCALLARM(addr, loc, seg, sym)
				case R_ADDRARM64:
					if symbol.Kind != STEXT {
						seg.errors += fmt.Sprintf("impossible!Sym:%s locate not in code segment!\n", sym.Name)
					}
					relocateADRP(seg.codeByte[loc.Offset:], loc, seg, addr, sym.Name)
				case R_ADDR:
					address := uintptr(int(addr) + loc.Add)
					putAddress(relocByte[loc.Offset:], uint64(address))
				case R_CALLIND:
					//nothing todo
				case R_ADDROFF, R_WEAKADDROFF, R_METHODOFF:
					if symbol.Kind == STEXT {
						seg.errors += fmt.Sprintf("impossible!Sym:%s locate on code segment!\n", sym.Name)
					}
					offset := int(addr) - seg.codeBase + loc.Add
					binary.LittleEndian.PutUint32(seg.codeByte[seg.codeLen+loc.Offset:], uint32(offset))
				default:
					seg.errors += fmt.Sprintf("unknown reloc type:%d sym:%s\n", loc.Type, sym.Name)
				}
			}

		}
	}
}

func addFuncTab(module *moduledata, i int, pclnOff *int, codereloc *CodeReloc, seg *segment, symPtr map[string]uintptr) {
	module.ftab[i].entry = uintptr(seg.symAddrs[codereloc.funcdata[i].Name])

	offset := alignof(*pclnOff, PtrSize)
	module.ftab[i].funcoff = uintptr(offset)
	funcdata := codereloc.funcdata[i]
	funcdata.entry = module.ftab[i].entry

	data := make([]uintptr, len(funcdata.Func.FuncData))
	for k, symbol := range funcdata.Func.FuncData {
		if codereloc.stkmaps[symbol.Sym.Name] != nil {
			data[k] = (uintptr)(unsafe.Pointer(&(codereloc.stkmaps[symbol.Sym.Name][0])))
		} else {
			data[k] = (uintptr)(0)
		}
	}

	addStackObject(codereloc, &funcdata, seg, symPtr)
	addDeferReturn(codereloc, &funcdata, seg)

	copy2Slice(module.pclntable[offset:], uintptr(unsafe.Pointer(&funcdata._func)), _FuncSize)
	offset += _FuncSize

	pcln := uint32(funcdata.pcln) + uint32(funcdata.Func.PCLine.Size)
	for k := 0; k < len(funcdata.Func.PCData); k++ {
		binary.LittleEndian.PutUint32(module.pclntable[offset:], pcln)
		pcln += uint32(funcdata.Func.PCData[k].Size)
		offset += Uint32Size
	}

	offset = alignof(offset, PtrSize)
	funcDataSize := int(PtrSize * funcdata.nfuncdata)
	copy2Slice(module.pclntable[offset:], uintptr(unsafe.Pointer(&data[0])), funcDataSize)
	offset += funcDataSize

	*pclnOff = offset
}

func buildModule(codereloc *CodeReloc, symPtr map[string]uintptr, codeModule *CodeModule, seg *segment) {
	module := moduledata{}
	module.ftab = make([]functab, len(codereloc.funcdata))
	pclnOff := len(codereloc.pclntable)
	module.pclntable = make([]byte, 4*len(codereloc.pclntable))
	copy(module.pclntable, codereloc.pclntable)
	module.minpc = uintptr(seg.codeBase)
	module.maxpc = uintptr(seg.dataBase)
	module.filetab = codereloc.filetab
	module.typemap = make(map[typeOff]uintptr)
	module.types = uintptr(seg.codeBase)
	module.etypes = uintptr(seg.codeBase + seg.maxLength)
	module.text = uintptr(seg.codeBase)
	module.etext = uintptr(seg.codeBase + len(codereloc.code))
	codeModule.stkmaps = codereloc.stkmaps // hold reference
	for i := range module.ftab {
		addFuncTab(&module, i, &pclnOff, codereloc, seg, symPtr)
	}
	pclnOff = alignof(pclnOff, PtrSize)
	module.findfunctab = (uintptr)(unsafe.Pointer(&module.pclntable[pclnOff]))
	copy2Slice(module.pclntable[pclnOff:], module.findfunctab, len(codereloc.pcfunc)*FindFuncBucketSize)
	pclnOff += len(codereloc.pcfunc) * FindFuncBucketSize
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

	codeModule.codeByte = seg.codeByte
}

func Load(codereloc *CodeReloc, symPtr map[string]uintptr) (*CodeModule, error) {
	seg := segment{symAddrs: make(map[string]uintptr)}
	seg.codeLen = len(codereloc.code)
	seg.dataLen = len(codereloc.data)
	seg.maxLength = (seg.codeLen + seg.dataLen) * 2
	codeByte, err := Mmap(seg.maxLength)
	if err != nil {
		return nil, err
	}
	seg.codeByte = codeByte

	codeModule := CodeModule{
		Syms:  make(map[string]uintptr),
		itabs: make(map[string]*itabSym),
	}

	seg.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	seg.dataBase = seg.codeBase + len(codereloc.code)
	seg.offset = seg.codeLen + seg.dataLen
	//static_tmp is 0, golang compile not allocate memory.
	seg.offset += IntSize
	copy(seg.codeByte, codereloc.code)
	copy(seg.codeByte[seg.codeLen:], codereloc.data)

	addSymAddrs(codereloc, symPtr, &codeModule, &seg)
	relocate(codereloc, symPtr, &codeModule, &seg)
	buildModule(codereloc, symPtr, &codeModule, &seg)
	relocateItab(codereloc, &codeModule, &seg)

	if len(seg.errors) > 0 {
		return &codeModule, errors.New(seg.errors)
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
	removeModule(cm.module)
	modulesLock.Unlock()
	Munmap(cm.codeByte)
}
