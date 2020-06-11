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

// copy from $GOROOT/src/cmd/internal/goobj/read.go type Sym struct
type Sym struct {
	Name   string
	Kind   int
	Offset int
	Func   *goobj.Func
	Reloc  []Reloc
}

// copy from $GOROOT/src/cmd/internal/goobj/read.go type Reloc struct
type Reloc struct {
	Offset   int
	Sym      *Sym
	Size     int
	Type     int
	Add      int
	DataSize int64
}

// ourself defined struct
// code segment
type segment struct {
	codeByte  []byte
	codeBase  int
	dataBase  int
	dataLen   int
	codeLen   int
	maxLength int
	offset    int
}

type CodeReloc struct {
	code      []byte
	data      []byte
	symMap    map[string]*Sym
	stkmaps   map[string][]byte
	fileMap   map[string]int
	filetab   []uint32
	pclntable []byte
	pcfunc    []findfuncbucket
	_func     []_func
	Arch      string
}

type CodeModule struct {
	segment
	Syms    map[string]uintptr
	module  *moduledata
	stkmaps map[string][]byte
	itabs   map[string]*itabSym
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

var (
	modules       = make(map[interface{}]bool)
	modulesLock   sync.Mutex
	x86moduleHead = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x1, PtrSize}
	armmoduleHead = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x4, PtrSize}
)

func relocSym(codereloc *CodeReloc, name string, objSymMap map[string]objSym) (*Sym, error) {
	if symbol, ok := codereloc.symMap[name]; ok {
		return symbol, nil
	}
	objsym := objSymMap[name].sym
	symbol := &Sym{Name: objsym.Name, Kind: int(objsym.Kind), Func: objsym.Func}
	codereloc.symMap[symbol.Name] = symbol

	code := make([]byte, objsym.Data.Size)
	_, err := objSymMap[name].file.ReadAt(code, objsym.Data.Offset)
	if err != nil {
		return nil, err
	}
	switch symbol.Kind {
	case STEXT:
		symbol.Offset = len(codereloc.code)
		codereloc.code = append(codereloc.code, code...)
		bytearrayAlign(&codereloc.code, PtrSize)
		if err := readFuncData(codereloc, objSymMap[name], objSymMap, symbol.Offset); err != nil {
			return nil, err
		}
	default:
		symbol.Offset = len(codereloc.data)
		codereloc.data = append(codereloc.data, code...)
		bytearrayAlign(&codereloc.data, PtrSize)
	}

	for _, loc := range objsym.Reloc {
		reloc := Reloc{
			Offset:   int(loc.Offset) + symbol.Offset,
			Sym:      &Sym{Name: loc.Sym.Name, Offset: INVALID_OFFSET},
			Type:     int(loc.Type),
			Size:     int(loc.Size),
			Add:      int(loc.Add),
			DataSize: -1}
		if _, ok := objSymMap[loc.Sym.Name]; ok {
			if reloc.Sym, err = relocSym(codereloc, loc.Sym.Name, objSymMap); err != nil {
				return nil, err
			}
			if objSymMap[loc.Sym.Name].sym.Data.Size == 0 && loc.Size > 0 {
				reloc.DataSize = 0
			}
		} else {
			if loc.Type == R_TLS_LE {
				reloc.Sym.Name = TLSNAME
				reloc.Sym.Offset = int(loc.Offset)
			}
			if loc.Type == R_CALLIND {
				reloc.Sym.Offset = 0
				reloc.Sym.Name = R_CALLIND_NAME
			}
			if strings.HasPrefix(loc.Sym.Name, TYPE_IMPORTPATH_PREFIX) {
				path := strings.Trim(strings.TrimLeft(loc.Sym.Name, TYPE_IMPORTPATH_PREFIX), ".")
				reloc.Sym.Offset = len(codereloc.data)
				codereloc.data = append(codereloc.data, path...)
				codereloc.data = append(codereloc.data, ZERO_BYTE)
			}
			codereloc.symMap[reloc.Sym.Name] = reloc.Sym
		}
		symbol.Reloc = append(symbol.Reloc, reloc)
	}
	return symbol, nil
}

func relocateADRP(mCode []byte, loc Reloc, segment *segment, symAddr uintptr) {
	offset := int64(symAddr) + int64(loc.Add) - ((int64(segment.codeBase) + int64(loc.Offset)) &^ 0xFFF)
	//overflow
	if offset > 0xFFFFFFFF || offset <= -0x100000000 {
		//low:	MOV reg imm
		//high: MOVK reg imm LSL#16
		value := uint64(0xF2A00000D2800000)
		addr := binary.LittleEndian.Uint32(mCode)
		low := uint32(value & 0xFFFFFFFF)
		high := uint32(value >> 32)
		low = ((addr & 0x1F) | low) | ((uint32(symAddr) & 0xFFFF) << 5)
		high = ((addr & 0x1F) | high) | (uint32(symAddr) >> 16 << 5)
		binary.LittleEndian.PutUint64(mCode, uint64(low)|(uint64(high)<<32))
	} else {
		// 2bit + 19bit + low(12bit) = 33bit
		low := (uint32((offset>>12)&3) << 29) | (uint32((offset>>12>>2)&0x7FFFF) << 5)
		high := (uint32(offset&0xFFF) << 10)
		value := binary.LittleEndian.Uint64(mCode)
		value = (uint64(uint32(value>>32)|high) << 32) | uint64(uint32(value&0xFFFFFFFF)|low)
		binary.LittleEndian.PutUint64(mCode, value)
	}
}

func addSymbolMap(codereloc *CodeReloc, symPtr map[string]uintptr, codeModule *CodeModule) (symbolMap map[string]uintptr, err error) {
	symbolMap = make(map[string]uintptr)
	segment := &codeModule.segment
	for name, sym := range codereloc.symMap {
		if sym.Offset == INVALID_OFFSET {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			} else {
				symbolMap[name] = INVALID_HANDLE_VALUE
				err = errors.New(fmt.Sprintf("unresolve external:%s", sym.Name))
			}
		} else if sym.Name == TLSNAME {
			regTLS(symbolMap, sym.Offset)
		} else if sym.Kind == STEXT {
			symbolMap[name] = uintptr(codereloc.symMap[name].Offset + segment.codeBase)
			codeModule.Syms[sym.Name] = uintptr(symbolMap[name])
		} else if strings.HasPrefix(sym.Name, ITAB_PREFIX) {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			}
		} else {
			symbolMap[name] = uintptr(codereloc.symMap[name].Offset + segment.dataBase)
			if strings.HasPrefix(sym.Name, TYPE_PREFIX) {
				if ptr, ok := symPtr[sym.Name]; ok {
					symbolMap[name] = ptr
				}
			}
		}
	}
	return symbolMap, err
}

func relocateItab(codereloc *CodeReloc, codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	for itabName, iter := range codeModule.itabs {
		symbol := codereloc.symMap[itabName]
		inter := symbolMap[symbol.Reloc[0].Sym.Name]
		typ := symbolMap[symbol.Reloc[1].Sym.Name]
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
					offset := int(address) - (segment.codeBase + iter.Offset + iter.Size) + iter.Add
					if offset > 0x7FFFFFFF || offset < -0x80000000 {
						offset = segment.offset - (iter.Offset + iter.Size) + iter.Add
						binary.LittleEndian.PutUint32(segment.codeByte[iter.Offset:], uint32(offset))
						if segment.codeByte[iter.Offset-2] == x86amd64MOVcode {
							//!!!TRICK
							//because struct itab doesn't change after it adds into itab list, so
							//copy itab data instead of jump code
							copy2Slice(segment.codeByte[segment.offset:], address, ItabSize)
							segment.offset += ItabSize
						} else if segment.codeByte[iter.Offset-2] == x86amd64LEAcode {
							segment.codeByte[iter.Offset-2:][0] = x86amd64MOVcode
							putAddressAddOffset(segment.codeByte, &segment.offset, uint64(address))
						} else {
							err = errors.New(fmt.Sprintf("relocateItab: not support code:%v!", segment.codeByte[iter.Offset-2:iter.Offset]))
						}
					} else {
						binary.LittleEndian.PutUint32(segment.codeByte[iter.Offset:], uint32(offset))
					}
				case R_ADDRARM64:
					relocateADRP(segment.codeByte[iter.Offset:], iter.Reloc, segment, address)
				case R_ADDR:
					putAddress(segment.codeByte[iter.Offset:], uint64(int(address)+iter.Add))
				default:
					err = errors.New(fmt.Sprintf("unknown relocateItab type:%d Name:%s", iter.Type, itabName))
				}
			}
		}
	}
	return err
}

func relocateCALL(addr uintptr, loc Reloc, segment *segment, relocByte []byte, addrBase int) {
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		offset = (segment.codeBase + segment.offset) - (addrBase + loc.Offset + loc.Size)
		copy(segment.codeByte[segment.offset:], x86amd64JMPLcode)
		segment.offset += len(x86amd64JMPLcode)
		putAddressAddOffset(segment.codeByte, &segment.offset, uint64(addr)+uint64(loc.Add))
	}
	binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
}

func relocatePCREL(addr uintptr, loc Reloc, segment *segment, relocByte []byte, addrBase int) (err error) {
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		offset = (segment.codeBase + segment.offset) - (addrBase + loc.Offset + loc.Size)
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
			err = errors.New(fmt.Sprintf("not support code:%v!", relocByte[loc.Offset-2:loc.Offset]))
		}
		binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
		if opcode == x86amd64CMPLcode || opcode == x86amd64MOVcode {
			putAddressAddOffset(segment.codeByte, &segment.offset, uint64(segment.codeBase+segment.offset+PtrSize))
			if opcode == x86amd64CMPLcode {
				copy(segment.codeByte[segment.offset:], x86amd64replaceCMPLcode)
				segment.codeByte[segment.offset+0x0F] = relocByte[loc.Offset+loc.Size]
				segment.offset += len(x86amd64replaceCMPLcode)
				putAddressAddOffset(segment.codeByte, &segment.offset, uint64(addr))
			} else {
				copy(segment.codeByte[segment.offset:], x86amd64replaceMOVQcode)
				segment.codeByte[segment.offset+1] = regsiter
				copy2Slice(segment.codeByte[segment.offset+2:], addr, PtrSize)
				segment.offset += len(x86amd64replaceMOVQcode)
			}
			putAddressAddOffset(segment.codeByte, &segment.offset, uint64(addrBase+loc.Offset+loc.Size-loc.Add))
		} else {
			putAddressAddOffset(segment.codeByte, &segment.offset, uint64(addr))
		}
	} else {
		binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
	}
	return err
}

func relocteCALLARM(addr uintptr, loc Reloc, segment *segment) {
	add := loc.Add
	if loc.Type == R_CALLARM {
		add = int(signext24(int64(loc.Add&0xFFFFFF)) * 4)
	}
	offset := (int(addr) + add - (segment.codeBase + loc.Offset)) / 4
	if offset > 0x7FFFFF || offset < -0x800000 {
		segment.offset = alignof(segment.offset, PtrSize)
		off := uint32(segment.offset-loc.Offset) / 4
		if loc.Type == R_CALLARM {
			add = int(signext24(int64(loc.Add&0xFFFFFF)+2) * 4)
			off = uint32(segment.offset-loc.Offset-8) / 4
		}
		putUint24(segment.codeByte[loc.Offset:], off)
		if loc.Type == R_CALLARM64 {
			copy(segment.codeByte[segment.offset:], arm64code)
			segment.offset += len(arm64code)
		} else {
			copy(segment.codeByte[segment.offset:], armcode)
			segment.offset += len(armcode)
		}
		putAddressAddOffset(segment.codeByte, &segment.offset, uint64(int(addr)+add))
	} else {
		val := binary.LittleEndian.Uint32(segment.codeByte[loc.Offset:])
		if loc.Type == R_CALLARM {
			val |= uint32(offset) & 0x00FFFFFF
		} else {
			val |= uint32(offset) & 0x03FFFFFF
		}
		binary.LittleEndian.PutUint32(segment.codeByte[loc.Offset:], val)
	}
}

func relocate(codereloc *CodeReloc, codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	for _, symbol := range codereloc.symMap {
		for _, loc := range symbol.Reloc {
			addr := symbolMap[loc.Sym.Name]
			sym := loc.Sym
			relocByte := segment.codeByte[segment.codeLen:]
			addrBase := segment.dataBase
			if symbol.Kind == STEXT {
				addrBase = segment.codeBase
				relocByte = segment.codeByte
			}
			//static_tmp is 0, golang compile not allocate memory.
			if loc.DataSize == 0 && loc.Size > 0 {
				if loc.Size <= IntSize {
					addr = uintptr(segment.codeBase + segment.codeLen + segment.dataLen)
				} else {
					err = errors.New(fmt.Sprintf("Symbol:%s size:%d>IntSize:%d", sym.Name, loc.Size, IntSize))
				}
			}
			if addr == INVALID_HANDLE_VALUE {
				//nothing todo
			} else if addr == 0 && strings.HasPrefix(sym.Name, ITAB_PREFIX) {
				codeModule.itabs[sym.Name] = &itabSym{Reloc: loc, inter: nil, typ: nil, ptr: nil}
			} else {
				switch loc.Type {
				case R_TLS_LE:
					binary.LittleEndian.PutUint32(segment.codeByte[loc.Offset:], uint32(symbolMap[TLSNAME]))
				case R_CALL:
					relocateCALL(addr, loc, segment, relocByte, addrBase)
				case R_PCREL:
					err = relocatePCREL(addr, loc, segment, relocByte, addrBase)
				case R_CALLARM, R_CALLARM64:
					relocteCALLARM(addr, loc, segment)
				case R_ADDRARM64:
					if symbol.Kind != STEXT {
						err = errors.New(fmt.Sprintf("impossible!Sym:%s locate not in code segment!", sym.Name))
					}
					relocateADRP(segment.codeByte[loc.Offset:], loc, segment, addr)
				case R_ADDR:
					address := uintptr(int(addr) + loc.Add)
					putAddress(relocByte[loc.Offset:], uint64(address))
				case R_CALLIND:
					//nothing todo
				case R_ADDROFF, R_WEAKADDROFF, R_METHODOFF:
					if symbol.Kind == STEXT {
						err = errors.New(fmt.Sprintf("impossible!Sym:%s locate on code segment!", sym.Name))
					}
					offset := int(addr) - segment.codeBase + loc.Add
					binary.LittleEndian.PutUint32(segment.codeByte[segment.codeLen+loc.Offset:], uint32(offset))
				default:
					err = errors.New(fmt.Sprintf("unknown reloc type:%d sym:%s", loc.Type, sym.Name))
				}
			}

		}
	}
	return err
}

func addFuncTab(module *moduledata, index int, pclnOff *int, codereloc *CodeReloc, symbolMap map[string]uintptr) (err error) {
	funcname := gostringnocopy(&codereloc.pclntable[codereloc._func[index].nameoff])
	Func := codereloc.symMap[funcname].Func
	module.ftab[index].entry = uintptr(symbolMap[funcname])

	offset := alignof(*pclnOff, PtrSize)
	module.ftab[index].funcoff = uintptr(offset)
	_func := codereloc._func[index]
	_func.entry = module.ftab[index].entry

	data := make([]uintptr, len(Func.FuncData))
	for k, symbol := range Func.FuncData {
		if codereloc.stkmaps[symbol.Sym.Name] != nil {
			data[k] = (uintptr)(unsafe.Pointer(&(codereloc.stkmaps[symbol.Sym.Name][0])))
		} else {
			data[k] = (uintptr)(0)
		}
	}

	if err = addStackObject(codereloc, funcname, symbolMap); err != nil {
		return err
	}
	if err = addDeferReturn(codereloc, &_func, funcname); err != nil {
		return err
	}

	copy2Slice(module.pclntable[offset:], uintptr(unsafe.Pointer(&_func)), _FuncSize)
	offset += _FuncSize

	pcln := uint32(_func.pcln) + uint32(Func.PCLine.Size)
	for k := 0; k < len(Func.PCData); k++ {
		binary.LittleEndian.PutUint32(module.pclntable[offset:], pcln)
		pcln += uint32(Func.PCData[k].Size)
		offset += Uint32Size
	}

	offset = alignof(offset, PtrSize)
	funcDataSize := int(PtrSize * _func.nfuncdata)
	copy2Slice(module.pclntable[offset:], uintptr(unsafe.Pointer(&data[0])), funcDataSize)
	offset += funcDataSize

	*pclnOff = offset
	return err
}

func buildModule(codereloc *CodeReloc, codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	module := moduledata{}
	module.ftab = make([]functab, len(codereloc._func))
	pclnOff := len(codereloc.pclntable)
	module.pclntable = make([]byte, 4*len(codereloc.pclntable))
	copy(module.pclntable, codereloc.pclntable)
	module.minpc = uintptr(segment.codeBase)
	module.maxpc = uintptr(segment.dataBase)
	module.filetab = codereloc.filetab
	module.typemap = make(map[typeOff]uintptr)
	module.types = uintptr(segment.codeBase)
	module.etypes = uintptr(segment.codeBase + segment.offset)
	module.text = uintptr(segment.codeBase)
	module.etext = uintptr(segment.codeBase + len(codereloc.code))
	codeModule.stkmaps = codereloc.stkmaps // hold reference
	for index := range module.ftab {
		if err = addFuncTab(&module, index, &pclnOff, codereloc, symbolMap); err != nil {
			return err
		}
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

	return err
}

func Load(codereloc *CodeReloc, symPtr map[string]uintptr) (codeModule *CodeModule, err error) {
	codeModule = &CodeModule{
		Syms:  make(map[string]uintptr),
		itabs: make(map[string]*itabSym),
	}
	codeModule.codeLen = len(codereloc.code)
	codeModule.dataLen = len(codereloc.data)
	codeModule.maxLength = (codeModule.codeLen + codeModule.dataLen) * 2
	codeByte, err := Mmap(codeModule.maxLength)
	if err != nil {
		return nil, err
	}

	codeModule.codeByte = codeByte
	codeModule.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	codeModule.dataBase = codeModule.codeBase + len(codereloc.code)
	codeModule.offset = codeModule.codeLen + codeModule.dataLen
	//static_tmp is 0, golang compile not allocate memory.
	codeModule.offset += IntSize
	copy(codeModule.codeByte, codereloc.code)
	copy(codeModule.codeByte[codeModule.codeLen:], codereloc.data)

	var symbolMap map[string]uintptr
	if symbolMap, err = addSymbolMap(codereloc, symPtr, codeModule); err == nil {
		if err = relocate(codereloc, codeModule, symbolMap); err == nil {
			if err = buildModule(codereloc, codeModule, symbolMap); err == nil {
				if err = relocateItab(codereloc, codeModule, symbolMap); err == nil {
					return codeModule, err
				}
			}
		}
	}
	return nil, err
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
