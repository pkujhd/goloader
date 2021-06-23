package goloader

import (
	"encoding/binary"
	"errors"
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

type Linker struct {
	code         []byte
	data         []byte
	symMap       map[string]*Sym
	objsymbolMap map[string]*ObjSymbol
	stkmaps      map[string][]byte
	namemap      map[string]int
	filetab      []uint32
	pclntable    []byte
	pcfunc       []findfuncbucket
	_func        []_func
	initFuncs    []string
	Arch         string
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

func (linker *Linker) addSymbols() error {
	//static_tmp is 0, golang compile not allocate memory.
	linker.data = append(linker.data, make([]byte, IntSize)...)
	for _, objSym := range linker.objsymbolMap {
		if objSym.Kind == STEXT && objSym.DupOK == false {
			_, err := linker.addSymbol(objSym.Name)
			if err != nil {
				return err
			}
		}
		if objSym.Kind == SNOPTRDATA {
			_, err := linker.addSymbol(objSym.Name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (linker *Linker) addSymbol(name string) (symbol *Sym, err error) {
	if symbol, ok := linker.symMap[name]; ok {
		return symbol, nil
	}
	objsym := linker.objsymbolMap[name]
	symbol = &Sym{Name: objsym.Name, Kind: int(objsym.Kind)}
	linker.symMap[symbol.Name] = symbol

	switch symbol.Kind {
	case STEXT:
		symbol.Offset = len(linker.code)
		linker.code = append(linker.code, objsym.Data...)
		bytearrayAlign(&linker.code, PtrSize)
		symbol.Func = &Func{}
		if err := linker.readFuncData(linker.objsymbolMap[name], symbol.Offset); err != nil {
			return nil, err
		}
	default:
		symbol.Offset = len(linker.data)
		linker.data = append(linker.data, objsym.Data...)
		bytearrayAlign(&linker.data, PtrSize)
	}

	for _, loc := range objsym.Reloc {
		reloc := loc
		reloc.Offset = reloc.Offset + symbol.Offset
		if _, ok := linker.objsymbolMap[reloc.Sym.Name]; ok {
			reloc.Sym, err = linker.addSymbol(reloc.Sym.Name)
			if err != nil {
				return nil, err
			}
			if len(linker.objsymbolMap[reloc.Sym.Name].Data) == 0 && reloc.Size > 0 {
				//static_tmp is 0, golang compile not allocate memory.
				//goloader add IntSize bytes on linker.data[0]
				if int(reloc.Size) <= IntSize {
					reloc.Sym.Offset = 0
				} else {
					return nil, fmt.Errorf("Symbol:%s size:%d>IntSize:%d", reloc.Sym.Name, reloc.Size, IntSize)
				}
			}
		} else {
			if reloc.Type == R_TLS_LE {
				reloc.Sym.Name = TLSNAME
				reloc.Sym.Offset = loc.Offset
			}
			if reloc.Type == R_CALLIND {
				reloc.Sym.Offset = 0
			}
			if strings.HasPrefix(reloc.Sym.Name, TypeImportPathPrefix) {
				path := strings.Trim(strings.TrimLeft(reloc.Sym.Name, TypeImportPathPrefix), ".")
				reloc.Sym.Offset = len(linker.data)
				linker.data = append(linker.data, path...)
				linker.data = append(linker.data, ZeroByte)
			}
			if ispreprocesssymbol(reloc.Sym.Name) {
				bytes := make([]byte, UInt64Size)
				if err := preprocesssymbol(reloc.Sym.Name, bytes); err != nil {
					return nil, err
				} else {
					reloc.Sym.Offset = len(linker.data)
					linker.data = append(linker.data, bytes...)
				}
			}
			if _, ok := linker.symMap[reloc.Sym.Name]; !ok {
				//golang1.8, some function generates more than one (MOVQ (TLS), CX)
				//so when same name symbol in linker.symMap, do not update it
				linker.symMap[reloc.Sym.Name] = reloc.Sym
			}
		}
		symbol.Reloc = append(symbol.Reloc, reloc)
	}
	return symbol, nil
}

func (linker *Linker) readFuncData(symbol *ObjSymbol, codeLen int) (err error) {
	x := codeLen
	b := x / pcbucketsize
	i := x % pcbucketsize / (pcbucketsize / nsub)
	for lb := b - len(linker.pcfunc); lb >= 0; lb-- {
		linker.pcfunc = append(linker.pcfunc, findfuncbucket{
			idx: uint32(256 * len(linker.pcfunc))})
	}
	bucket := &linker.pcfunc[b]
	bucket.subbuckets[i] = byte(len(linker._func) - int(bucket.idx))

	pcFileHead := make([]byte, 32)
	pcFileHeadSize := binary.PutUvarint(pcFileHead, uint64(len(linker.filetab))<<1)
	for _, fileName := range symbol.Func.File {
		if offset, ok := linker.namemap[fileName]; !ok {
			linker.filetab = append(linker.filetab, (uint32)(len(linker.pclntable)))
			linker.namemap[fileName] = len(linker.pclntable)
			fileName = strings.TrimLeft(fileName, FileSymPrefix)
			linker.pclntable = append(linker.pclntable, []byte(fileName)...)
			linker.pclntable = append(linker.pclntable, ZeroByte)
		} else {
			linker.filetab = append(linker.filetab, uint32(offset))
		}
	}

	nameOff := len(linker.pclntable)
	if offset, ok := linker.namemap[symbol.Name]; !ok {
		linker.namemap[symbol.Name] = len(linker.pclntable)
		linker.pclntable = append(linker.pclntable, []byte(symbol.Name)...)
		linker.pclntable = append(linker.pclntable, ZeroByte)
	} else {
		nameOff = offset
	}

	pcspOff := len(linker.pclntable)
	linker.pclntable = append(linker.pclntable, symbol.Func.PCSP...)

	pcfileOff := len(linker.pclntable)
	linker.pclntable = append(linker.pclntable, pcFileHead[:pcFileHeadSize-1]...)
	linker.pclntable = append(linker.pclntable, symbol.Func.PCFile...)

	pclnOff := len(linker.pclntable)
	linker.pclntable = append(linker.pclntable, symbol.Func.PCLine...)

	_func := init_func(symbol, nameOff, pcspOff, pcfileOff, pclnOff)
	Func := linker.symMap[symbol.Name].Func
	for _, pcdata := range symbol.Func.PCData {
		Func.PCData = append(Func.PCData, uint32(len(linker.pclntable)))
		linker.pclntable = append(linker.pclntable, pcdata...)
	}

	for _, name := range symbol.Func.FuncData {
		if _, ok := linker.stkmaps[name]; !ok {
			if gcobj, ok := linker.objsymbolMap[name]; ok {
				linker.stkmaps[name] = gcobj.Data
			} else if len(name) == 0 {
				linker.stkmaps[name] = nil
			} else {
				return errors.New("unknown gcobj:" + name)
			}
		}
		if linker.stkmaps[name] != nil {
			Func.FuncData = append(Func.FuncData, (uintptr)(unsafe.Pointer(&(linker.stkmaps[name][0]))))
		} else {
			Func.FuncData = append(Func.FuncData, (uintptr)(0))
		}
	}

	if err = linker.addInlineTree(&_func, symbol); err != nil {
		return err
	}

	grow(&linker.pclntable, alignof(len(linker.pclntable), PtrSize))
	linker._func = append(linker._func, _func)

	for _, name := range symbol.Func.FuncData {
		if _, ok := linker.objsymbolMap[name]; ok {
			linker.addSymbol(name)
		}
	}
	return
}

func (linker *Linker) addSymbolMap(symPtr map[string]uintptr, codeModule *CodeModule) (symbolMap map[string]uintptr, err error) {
	symbolMap = make(map[string]uintptr)
	segment := &codeModule.segment
	for name, sym := range linker.symMap {
		if sym.Offset == InvalidOffset {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			} else {
				symbolMap[name] = InvalidHandleValue
				return nil, fmt.Errorf("unresolve external:%s", sym.Name)
			}
		} else if sym.Name == TLSNAME {
			//nothing todo
		} else if sym.Kind == STEXT {
			symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.codeBase)
			codeModule.Syms[sym.Name] = uintptr(symbolMap[name])
		} else if strings.HasPrefix(sym.Name, ItabPrefix) {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			}
		} else {
			if _, ok := symPtr[name]; !ok {
				symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
			} else {
				symbolMap[name] = symPtr[name]
				if strings.HasPrefix(name, MainPkgPrefix) || strings.HasPrefix(name, TypePrefix) {
					symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
				}
			}
		}
	}
	return symbolMap, err
}

func relocateADRP(mCode []byte, loc Reloc, segment *segment, symAddr uintptr) {
	offset := uint64(int64(symAddr) + int64(loc.Add) - ((int64(segment.codeBase) + int64(loc.Offset)) &^ 0xFFF))
	//overflow
	if offset > 0xFFFFFFFF {
		if symAddr < 0xFFFFFFFF {
			addr := binary.LittleEndian.Uint32(mCode)
			//low:	MOV reg imm
			low := uint32(0xD2800000)
			//high: MOVK reg imm LSL#16
			high := uint32(0xF2A00000)
			low = ((addr & 0x1F) | low) | ((uint32(symAddr) & 0xFFFF) << 5)
			high = ((addr & 0x1F) | high) | (uint32(symAddr) >> 16 << 5)
			binary.LittleEndian.PutUint64(mCode, uint64(low)|(uint64(high)<<32))
		} else {
			addr := binary.LittleEndian.Uint32(mCode)
			blcode := binary.LittleEndian.Uint32(arm64BLcode)
			blcode |= ((uint32(segment.offset) - uint32(loc.Offset)) >> 2) & 0x01FFFFFF
			if segment.offset-loc.Offset < 0 {
				blcode |= 0x02000000
			}
			binary.LittleEndian.PutUint32(mCode, blcode)
			//low: MOV reg imm
			llow := uint32(0xD2800000)
			//lhigh: MOVK reg imm LSL#16
			lhigh := uint32(0xF2A00000)
			//llow: MOVK reg imm LSL#32
			hlow := uint32(0xF2C00000)
			//lhigh: MOVK reg imm LSL#48
			hhigh := uint32(0xF2E00000)
			llow = ((addr & 0x1F) | llow) | ((uint32(symAddr) & 0xFFFF) << 5)
			lhigh = ((addr & 0x1F) | lhigh) | (uint32(symAddr) >> 16 << 5)
			putAddressAddOffset(segment.codeByte, &segment.offset, uint64(llow)|(uint64(lhigh)<<32))
			hlow = ((addr & 0x1F) | hlow) | uint32(((uint64(symAddr)>>32)&0xFFFF)<<5)
			hhigh = ((addr & 0x1F) | hhigh) | uint32((uint64(symAddr)>>48)<<5)
			putAddressAddOffset(segment.codeByte, &segment.offset, uint64(hlow)|(uint64(hhigh)<<32))
			blcode = binary.LittleEndian.Uint32(arm64BLcode)
			blcode |= ((uint32(loc.Offset) - uint32(segment.offset) + 8) >> 2) & 0x01FFFFFF
			if loc.Offset-segment.offset+8 < 0 {
				blcode |= 0x02000000
			}
			binary.LittleEndian.PutUint32(segment.codeByte[segment.offset:], blcode)
			segment.offset += Uint32Size
		}
	} else {
		// 2bit + 19bit + low(12bit) = 33bit
		low := (uint32((offset>>12)&3) << 29) | (uint32((offset>>12>>2)&0x7FFFF) << 5)
		high := (uint32(offset&0xFFF) << 10)
		value := binary.LittleEndian.Uint64(mCode)
		value = (uint64(uint32(value>>32)|high) << 32) | uint64(uint32(value&0xFFFFFFFF)|low)
		binary.LittleEndian.PutUint64(mCode, value)
	}
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

func (linker *Linker) relocate(codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	for _, symbol := range linker.symMap {
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
					if _, ok := symbolMap[TLSNAME]; !ok {
						regTLS(symbolMap, segment.codeByte[symbol.Offset:loc.Offset])
					}
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

func (linker *Linker) addFuncTab(module *moduledata, _func *_func, symbolMap map[string]uintptr) (err error) {
	funcname := gostringnocopy(&linker.pclntable[_func.nameoff])
	_func.entry = uintptr(symbolMap[funcname])
	Func := linker.symMap[funcname].Func

	if err = linker.addStackObject(funcname, symbolMap); err != nil {
		return err
	}
	if err = linker.addDeferReturn(_func); err != nil {
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

func (linker *Linker) buildModule(codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	module := codeModule.module
	module.pclntable = append(module.pclntable, linker.pclntable...)
	module.minpc = uintptr(segment.codeBase)
	module.maxpc = uintptr(segment.dataBase)
	module.types = uintptr(segment.codeBase)
	module.etypes = uintptr(segment.codeBase + segment.offset)
	module.text = uintptr(segment.codeBase)
	module.etext = uintptr(segment.codeBase + len(linker.code))
	codeModule.stkmaps = linker.stkmaps // hold reference

	module.ftab = append(module.ftab, functab{funcoff: uintptr(len(module.pclntable)), entry: module.minpc})
	for index, _func := range linker._func {
		funcname := gostringnocopy(&linker.pclntable[_func.nameoff])
		module.ftab = append(module.ftab, functab{funcoff: uintptr(len(module.pclntable)), entry: uintptr(symbolMap[funcname])})
		if err = linker.addFuncTab(module, &(linker._func[index]), symbolMap); err != nil {
			return err
		}
	}
	module.ftab = append(module.ftab, functab{funcoff: uintptr(len(module.pclntable)), entry: module.maxpc})

	length := len(linker.pcfunc) * FindFuncBucketSize
	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&linker.pcfunc[0])), length)
	module.findfunctab = (uintptr)(unsafe.Pointer(&module.pclntable[len(module.pclntable)-length]))
	linker._buildModule(codeModule)

	modulesLock.Lock()
	addModule(codeModule)
	modulesLock.Unlock()
	additabs(codeModule.module)
	moduledataverify1(codeModule.module)

	return err
}

func Load(linker *Linker, symPtr map[string]uintptr) (codeModule *CodeModule, err error) {
	codeModule = &CodeModule{
		Syms:   make(map[string]uintptr),
		module: &moduledata{typemap: make(map[typeOff]uintptr)},
	}
	codeModule.codeLen = len(linker.code)
	codeModule.dataLen = len(linker.data)
	codeModule.maxLength = alignof((codeModule.codeLen+codeModule.dataLen)*2, PageSize)
	codeByte, err := Mmap(codeModule.maxLength)
	if err != nil {
		return nil, err
	}

	codeModule.codeByte = codeByte
	codeModule.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	codeModule.dataBase = codeModule.codeBase + len(linker.code)
	codeModule.offset = codeModule.codeLen + codeModule.dataLen
	copy(codeModule.codeByte, linker.code)
	copy(codeModule.codeByte[codeModule.codeLen:], linker.data)

	var symbolMap map[string]uintptr
	if symbolMap, err = linker.addSymbolMap(symPtr, codeModule); err == nil {
		if err = linker.relocate(codeModule, symbolMap); err == nil {
			if err = linker.buildModule(codeModule, symbolMap); err == nil {
				if err = linker.doInitialize(codeModule, symbolMap); err == nil {
					return codeModule, err
				}
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
