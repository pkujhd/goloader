package goloader

import (
	"encoding/binary"
	"fmt"
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

type Func struct {
	PCData   []uint32
	FuncData []uintptr
}

// copy from $GOROOT/src/cmd/internal/goobj/read.go type Sym struct
type Sym struct {
	Name   string
	Kind   int
	Offset int
	Func   *Func
	Reloc  []Reloc
}

// copy from $GOROOT/src/cmd/internal/goobj/read.go type Reloc struct
type Reloc struct {
	Offset int
	Sym    *Sym
	Size   int
	Type   int
	Add    int
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
	namemap   map[string]int
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
}

type InlTreeNode struct {
	Parent   int64
	File     string
	Line     int64
	Func     string
	ParentPC int64
}

type FuncInfo struct {
	Args     uint32
	Locals   uint32
	FuncID   uint8
	PCSP     []byte
	PCFile   []byte
	PCLine   []byte
	PCInline []byte
	PCData   [][]byte
	File     []string
	FuncData []string
	InlTree  []InlTreeNode
}

type ObjSymbol struct {
	Name  string
	Kind  int    // kind of symbol
	DupOK bool   // are duplicate definitions okay?
	Size  int64  // size of corresponding data
	Data  []byte // memory image of symbol
	Reloc []Reloc
	Func  *FuncInfo // additional data for functions
}

var (
	modules     = make(map[interface{}]bool)
	modulesLock sync.Mutex
)

func relocSym(codereloc *CodeReloc, name string, objSymMap map[string]*ObjSymbol) (symbol *Sym, err error) {
	if symbol, ok := codereloc.symMap[name]; ok {
		return symbol, nil
	}
	objsym := objSymMap[name]
	symbol = &Sym{Name: objsym.Name, Kind: int(objsym.Kind)}
	codereloc.symMap[symbol.Name] = symbol

	switch symbol.Kind {
	case STEXT:
		symbol.Offset = len(codereloc.code)
		codereloc.code = append(codereloc.code, objsym.Data...)
		bytearrayAlign(&codereloc.code, PtrSize)
		symbol.Func = &Func{}
		if err := readFuncData(codereloc, objSymMap[name], objSymMap, symbol.Offset); err != nil {
			return nil, err
		}
	default:
		symbol.Offset = len(codereloc.data)
		codereloc.data = append(codereloc.data, objsym.Data...)
		bytearrayAlign(&codereloc.data, PtrSize)
	}

	for _, reloc := range objsym.Reloc {
		reloc.Offset = reloc.Offset + symbol.Offset
		if _, ok := objSymMap[reloc.Sym.Name]; ok {
			reloc.Sym, err = relocSym(codereloc, reloc.Sym.Name, objSymMap)
			if err != nil {
				return nil, err
			}
			if len(objSymMap[reloc.Sym.Name].Data) == 0 && reloc.Size > 0 {
				if int(reloc.Size) <= IntSize {
					reloc.Sym.Offset = 0
				} else {
					return nil, fmt.Errorf("Symbol:%s size:%d>IntSize:%d", reloc.Sym.Name, reloc.Size, IntSize)
				}
			}
		} else {
			if reloc.Type == R_TLS_LE {
				reloc.Sym.Name = TLSNAME
				reloc.Sym.Offset = reloc.Offset - symbol.Offset
			}
			if reloc.Type == R_CALLIND {
				reloc.Sym.Offset = 0
			}
			if strings.HasPrefix(reloc.Sym.Name, TypeImportPathPrefix) {
				path := strings.Trim(strings.TrimLeft(reloc.Sym.Name, TypeImportPathPrefix), ".")
				reloc.Sym.Offset = len(codereloc.data)
				codereloc.data = append(codereloc.data, path...)
				codereloc.data = append(codereloc.data, ZeroByte)
			}
			if ispreprocesssymbol(reloc.Sym.Name) {
				bytes := make([]byte, UInt64Size)
				if err := preprocesssymbol(reloc.Sym.Name, bytes); err != nil {
					return nil, err
				} else {
					reloc.Sym.Offset = len(codereloc.data)
					codereloc.data = append(codereloc.data, bytes...)
				}
			}
			if _, ok := codereloc.symMap[reloc.Sym.Name]; !ok {
				//golang1.8, some function generates more than one (MOVQ (TLS), CX)
				//so when same name symbol in codereloc.symMap, do not update it
				codereloc.symMap[reloc.Sym.Name] = reloc.Sym
			}
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
		if sym.Offset == InvalidOffset {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			} else {
				symbolMap[name] = InvalidHandleValue
				return nil, fmt.Errorf("unresolve external:%s", sym.Name)
			}
		} else if sym.Name == TLSNAME {
			regTLS(symbolMap, sym.Offset)
		} else if sym.Kind == STEXT {
			symbolMap[name] = uintptr(codereloc.symMap[name].Offset + segment.codeBase)
			codeModule.Syms[sym.Name] = uintptr(symbolMap[name])
		} else if strings.HasPrefix(sym.Name, ItabPrefix) {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			}
		} else {
			symbolMap[name] = uintptr(codereloc.symMap[name].Offset + segment.dataBase)
		}
	}
	return symbolMap, err
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
		regsiter := ZeroByte
		if opcode == x86amd64LEAcode {
			bytes[0] = x86amd64MOVcode
		} else if opcode == x86amd64MOVcode && loc.Size >= Uint32Size {
			regsiter = ((relocByte[loc.Offset-1] >> 3) & 0x7) | 0xb8
			copy(bytes, x86amd64JMPLcode)
		} else if opcode == x86amd64CMPLcode && loc.Size >= Uint32Size {
			copy(bytes, x86amd64JMPLcode)
		} else {
			return fmt.Errorf("not support code:%v!", relocByte[loc.Offset-2:loc.Offset])
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
			if addr == 0 && strings.HasPrefix(sym.Name, ItabPrefix) {
				addr = uintptr(segment.dataBase + loc.Sym.Offset)
				symbolMap[loc.Sym.Name] = addr
				codeModule.module.itablinks = append(codeModule.module.itablinks, (*itab)(adduintptr(uintptr(segment.dataBase), loc.Sym.Offset)))
			}
			if addr != InvalidHandleValue {
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
						err = fmt.Errorf("impossible!Sym:%s locate not in code segment!", sym.Name)
					}
					relocateADRP(segment.codeByte[loc.Offset:], loc, segment, addr)
				case R_ADDR:
					address := uintptr(int(addr) + loc.Add)
					putAddress(relocByte[loc.Offset:], uint64(address))
				case R_CALLIND:
					//nothing todo
				case R_ADDROFF, R_WEAKADDROFF, R_METHODOFF:
					if symbol.Kind == STEXT {
						err = fmt.Errorf("impossible!Sym:%s locate on code segment!", sym.Name)
					}
					offset := int(addr) - segment.codeBase + loc.Add
					if offset > 0x7FFFFFFF || offset < -0x80000000 {
						err = fmt.Errorf("symName:%s offset:%d is overflow!", sym.Name, offset)
					}
					binary.LittleEndian.PutUint32(segment.codeByte[segment.codeLen+loc.Offset:], uint32(offset))
				case R_USEIFACE:
					//nothing todo
				case R_USEIFACEMETHOD:
					//nothing todo
				case R_ADDRCUOFF:
					//nothing todo
				default:
					err = fmt.Errorf("unknown reloc type:%d sym:%s", loc.Type, sym.Name)
				}
			}
			if err != nil {
				return err
			}
		}
	}
	return err
}

func addFuncTab(module *moduledata, _func *_func, codereloc *CodeReloc, symbolMap map[string]uintptr) (err error) {
	funcname := gostringnocopy(&codereloc.pclntable[_func.nameoff])
	_func.entry = uintptr(symbolMap[funcname])
	Func := codereloc.symMap[funcname].Func

	if err = addStackObject(codereloc, funcname, symbolMap); err != nil {
		return err
	}
	if err = addDeferReturn(codereloc, _func); err != nil {
		return err
	}

	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(_func)), _FuncSize)

	if _func.npcdata > 0 {
		append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&(Func.PCData[0]))), Uint32Size*int(_func.npcdata))
	}

	grow(&module.pclntable, alignof(len(module.pclntable), PtrSize))
	if _func.nfuncdata > 0 {
		append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&Func.FuncData[0])), int(PtrSize*_func.nfuncdata))
	}

	return err
}

func buildModule(codereloc *CodeReloc, codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	module := codeModule.module
	module.pclntable = append(module.pclntable, codereloc.pclntable...)
	module.minpc = uintptr(segment.codeBase)
	module.maxpc = uintptr(segment.dataBase)
	module.types = uintptr(segment.codeBase)
	module.etypes = uintptr(segment.codeBase + segment.offset)
	module.text = uintptr(segment.codeBase)
	module.etext = uintptr(segment.codeBase + len(codereloc.code))
	codeModule.stkmaps = codereloc.stkmaps // hold reference

	module.ftab = append(module.ftab, functab{funcoff: uintptr(len(module.pclntable)), entry: module.minpc})
	for index, _func := range codereloc._func {
		funcname := gostringnocopy(&codereloc.pclntable[_func.nameoff])
		module.ftab = append(module.ftab, functab{funcoff: uintptr(len(module.pclntable)), entry: uintptr(symbolMap[funcname])})
		if err = addFuncTab(module, &(codereloc._func[index]), codereloc, symbolMap); err != nil {
			return err
		}
	}
	module.ftab = append(module.ftab, functab{funcoff: uintptr(len(module.pclntable)), entry: module.maxpc})

	length := len(codereloc.pcfunc) * FindFuncBucketSize
	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&codereloc.pcfunc[0])), length)
	module.findfunctab = (uintptr)(unsafe.Pointer(&module.pclntable[len(module.pclntable)-length]))
	_buildModule(codereloc, codeModule)

	modulesLock.Lock()
	addModule(codeModule)
	modulesLock.Unlock()
	additabs(codeModule.module)
	moduledataverify1(codeModule.module)

	return err
}

func Load(codereloc *CodeReloc, symPtr map[string]uintptr) (codeModule *CodeModule, err error) {
	codeModule = &CodeModule{
		Syms:   make(map[string]uintptr),
		module: &moduledata{typemap: make(map[typeOff]uintptr)},
	}
	codeModule.codeLen = len(codereloc.code)
	codeModule.dataLen = len(codereloc.data)
	codeModule.maxLength = alignof((codeModule.codeLen+codeModule.dataLen)*2, PageSize)
	codeByte, err := Mmap(codeModule.maxLength)
	if err != nil {
		return nil, err
	}

	codeModule.codeByte = codeByte
	codeModule.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	codeModule.dataBase = codeModule.codeBase + len(codereloc.code)
	codeModule.offset = codeModule.codeLen + codeModule.dataLen
	copy(codeModule.codeByte, codereloc.code)
	copy(codeModule.codeByte[codeModule.codeLen:], codereloc.data)

	var symbolMap map[string]uintptr
	if symbolMap, err = addSymbolMap(codereloc, symPtr, codeModule); err == nil {
		if err = relocate(codereloc, codeModule, symbolMap); err == nil {
			if err = buildModule(codereloc, codeModule, symbolMap); err == nil {
				return codeModule, err
			}
		}
	}
	return nil, err
}

func (cm *CodeModule) Unload() {
	removeitabs(cm.module)
	runtime.GC()
	modulesLock.Lock()
	removeModule(cm.module)
	modulesLock.Unlock()
	Munmap(cm.codeByte)
}
