package goloader

import (
	"cmd/objfile/objabi"
	"cmd/objfile/sys"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/pkujhd/goloader/mmap"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/reloctype"
	"github.com/pkujhd/goloader/objabi/symkind"
	"github.com/pkujhd/goloader/stackobject"
)

// ourself defined struct
// code segment
type segment struct {
	codeByte      []byte
	dataByte      []byte
	codeBase      int
	dataBase      int
	sumDataLen    int
	dataLen       int
	noptrdataLen  int
	bssLen        int
	noptrbssLen   int
	codeLen       int
	maxLengthCode int
	maxLengthData int
	codeOffset    int
	dataOffset    int
}

type Linker struct {
	code         []byte
	data         []byte
	noptrdata    []byte
	bss          []byte
	noptrbss     []byte
	cuFiles      []obj.CompilationUnitFiles
	symMap       map[string]*obj.Sym
	objsymbolMap map[string]*obj.ObjSymbol
	namemap      map[string]int
	fileNameMap  map[string]int
	cutab        []uint32
	filetab      []byte
	funcnametab  []byte
	functab      []byte
	pctab        []byte
	_func        []*_func
	initFuncs    []string
	Arch         *sys.Arch
}

type CodeModule struct {
	segment
	Syms   map[string]uintptr
	module *moduledata
	gcdata []byte
	gcbss  []byte
}

var (
	modules     = make(map[interface{}]bool)
	modulesLock sync.Mutex
)

// initialize Linker
func initLinker() *Linker {
	linker := &Linker{
		symMap:       make(map[string]*obj.Sym),
		objsymbolMap: make(map[string]*obj.ObjSymbol),
		namemap:      make(map[string]int),
		fileNameMap:  make(map[string]int),
	}
	head := make([]byte, unsafe.Sizeof(pcHeader{}))
	copy(head, obj.ModuleHeadx86)
	linker.functab = append(linker.functab, head...)
	linker.functab[len(obj.ModuleHeadx86)-1] = PtrSize
	return linker
}

func (linker *Linker) addSymbols() error {
	//static_tmp is 0, golang compile not allocate memory.
	linker.noptrdata = append(linker.noptrdata, make([]byte, IntSize)...)

	for _, cuFileSet := range linker.cuFiles {
		for _, fileName := range cuFileSet.Files {
			if offset, ok := linker.fileNameMap[fileName]; !ok {
				linker.cutab = append(linker.cutab, (uint32)(len(linker.filetab)))
				linker.fileNameMap[fileName] = len(linker.filetab)
				fileName = strings.TrimPrefix(fileName, FileSymPrefix)
				linker.filetab = append(linker.filetab, []byte(fileName)...)
				linker.filetab = append(linker.filetab, ZeroByte)
			} else {
				linker.cutab = append(linker.cutab, uint32(offset))
			}
		}
	}

	for _, objSym := range linker.objsymbolMap {
		if objSym.Kind == symkind.STEXT && objSym.DupOK == false {
			_, err := linker.addSymbol(objSym.Name)
			if err != nil {
				return err
			}
		} else if objSym.Kind == symkind.STEXT && objSym.DupOK {
			// This might be an asm func ABIWRAPPER. Check if one of its relocs points to itself
			// (the abi0 version of itself, without the .abiinternal suffix)
			isAsmWrapper := false

			if objSym.Func != nil && objSym.Func.FuncID == uint8(objabi.FuncID_wrapper) {
				for _, reloc := range objSym.Reloc {
					if reloc.Sym.Name+obj.ABIInternalSuffix == objSym.Name {
						// Relocation pointing at itself (the ABI0 ASM version)
						isAsmWrapper = true
					}
				}
			}
			if isAsmWrapper {
				// This wrapper's symbol has a suffix of .abiinternal to distinguish it from the abi0 ASM func
				_, err := linker.addSymbol(objSym.Name)
				if err != nil {
					return err
				}
			}
		}
		if objSym.Kind == symkind.SNOPTRDATA || objSym.Kind == symkind.SRODATA {
			_, err := linker.addSymbol(objSym.Name)
			if err != nil {
				return err
			}
		}
	}
	for _, sym := range linker.symMap {
		offset := 0
		switch sym.Kind {
		case symkind.SNOPTRDATA, symkind.SRODATA:
			if IsEnableStringMap() && strings.HasPrefix(sym.Name, TypeStringPrefix) {
				//nothing todo
			} else {
				offset += len(linker.data)
			}
		case symkind.SBSS:
			offset += len(linker.data) + len(linker.noptrdata)
		case symkind.SNOPTRBSS:
			offset += len(linker.data) + len(linker.noptrdata) + len(linker.bss)
		}
		sym.Offset += offset
		if offset != 0 {
			for index := range sym.Reloc {
				sym.Reloc[index].Offset += offset
			}
		}
	}
	return nil
}

func (linker *Linker) addSymbol(name string) (symbol *obj.Sym, err error) {
	if symbol, ok := linker.symMap[name]; ok {
		return symbol, nil
	}
	objsym := linker.objsymbolMap[name]
	symbol = &obj.Sym{Name: objsym.Name, Kind: objsym.Kind}
	linker.symMap[symbol.Name] = symbol

	switch symbol.Kind {
	case symkind.STEXT:
		symbol.Offset = len(linker.code)
		linker.code = append(linker.code, objsym.Data...)
		bytearrayAlign(&linker.code, PtrSize)
		symbol.Func = &obj.Func{}
		if err := linker.readFuncData(linker.objsymbolMap[name], symbol.Offset); err != nil {
			return nil, err
		}
	case symkind.SDATA:
		symbol.Offset = len(linker.data)
		linker.data = append(linker.data, objsym.Data...)
	case symkind.SNOPTRDATA, symkind.SRODATA:
		//because golang string assignment is pointer assignment, so store go.string constants
		//in a separate segment and not unload when module unload.
		if IsEnableStringMap() && strings.HasPrefix(symbol.Name, TypeStringPrefix) {
			if stringContainer.index+len(objsym.Data) > stringContainer.size {
				return nil, fmt.Errorf("overflow string container")
			}
			symbol.Offset = stringContainer.index
			copy(stringContainer.bytes[stringContainer.index:], objsym.Data)
			stringContainer.index += len(objsym.Data)
		} else {
			symbol.Offset = len(linker.noptrdata)
			linker.noptrdata = append(linker.noptrdata, objsym.Data...)
		}
	case symkind.SBSS:
		symbol.Offset = len(linker.bss)
		linker.bss = append(linker.bss, objsym.Data...)
	case symkind.SNOPTRBSS:
		symbol.Offset = len(linker.noptrbss)
		linker.noptrbss = append(linker.noptrbss, objsym.Data...)
	default:
		return nil, fmt.Errorf("invalid symbol:%s kind:%d", symbol.Name, symbol.Kind)
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
				//goloader add IntSize bytes on linker.noptrdata[0]
				if reloc.Size <= IntSize {
					reloc.Sym.Offset = 0
				} else {
					return nil, fmt.Errorf("Symbol: %s size: %d > IntSize: %d\n", reloc.Sym.Name, reloc.Size, IntSize)
				}
			}
		} else {
			if reloc.Type == reloctype.R_TLS_LE {
				reloc.Sym.Name = TLSNAME
				reloc.Sym.Offset = loc.Offset
			}
			if reloc.Type == reloctype.R_CALLIND {
				reloc.Sym.Offset = 0
			}
			_, exist := linker.symMap[reloc.Sym.Name]
			if strings.HasPrefix(reloc.Sym.Name, TypeImportPathPrefix) {
				if exist {
					reloc.Sym = linker.symMap[reloc.Sym.Name]
				} else {
					path := strings.Trim(strings.TrimPrefix(reloc.Sym.Name, TypeImportPathPrefix), ".")
					reloc.Sym.Kind = symkind.SNOPTRDATA
					reloc.Sym.Offset = len(linker.noptrdata)
					//name memory layout
					//name { tagLen(byte), len(uint16), str*}
					nameLen := []byte{0, 0, 0}
					binary.BigEndian.PutUint16(nameLen[1:], uint16(len(path)))
					linker.noptrdata = append(linker.noptrdata, nameLen...)
					linker.noptrdata = append(linker.noptrdata, path...)
					linker.noptrdata = append(linker.noptrdata, ZeroByte)
				}
			}
			if ispreprocesssymbol(reloc.Sym.Name) {
				bytes := make([]byte, UInt64Size)
				if err := preprocesssymbol(linker.Arch.ByteOrder, reloc.Sym.Name, bytes); err != nil {
					return nil, err
				} else {
					if exist {
						reloc.Sym = linker.symMap[reloc.Sym.Name]
					} else {
						reloc.Sym.Kind = symkind.SNOPTRDATA
						reloc.Sym.Offset = len(linker.noptrdata)
						linker.noptrdata = append(linker.noptrdata, bytes...)
					}
				}
			}
			if !exist {
				//golang1.8, some function generates more than one (MOVQ (TLS), CX)
				//so when same name symbol in linker.symMap, do not update it
				if reloc.Sym.Name != "" {
					linker.symMap[reloc.Sym.Name] = reloc.Sym
				}
			}
		}
		symbol.Reloc = append(symbol.Reloc, reloc)
	}

	if objsym.Type != EmptyString {
		if _, ok := linker.symMap[objsym.Type]; !ok {
			if _, ok := linker.objsymbolMap[objsym.Type]; !ok {
				linker.symMap[objsym.Type] = &obj.Sym{Name: objsym.Type, Offset: InvalidOffset}
			}
		}
	}
	return symbol, nil
}

func (linker *Linker) readFuncData(symbol *obj.ObjSymbol, codeLen int) (err error) {
	nameOff := len(linker.funcnametab)
	if offset, ok := linker.namemap[symbol.Name]; !ok {
		linker.namemap[symbol.Name] = len(linker.funcnametab)
		linker.funcnametab = append(linker.funcnametab, []byte(symbol.Name)...)
		linker.funcnametab = append(linker.funcnametab, ZeroByte)
	} else {
		nameOff = offset
	}

	pcspOff := len(linker.pctab)
	linker.pctab = append(linker.pctab, symbol.Func.PCSP...)

	pcfileOff := len(linker.pctab)
	linker.pctab = append(linker.pctab, symbol.Func.PCFile...)

	pclnOff := len(linker.pctab)
	linker.pctab = append(linker.pctab, symbol.Func.PCLine...)

	_func := initfunc(symbol, nameOff, pcspOff, pcfileOff, pclnOff, symbol.Func.CUOffset)
	linker._func = append(linker._func, &_func)
	Func := linker.symMap[symbol.Name].Func
	for _, pcdata := range symbol.Func.PCData {
		Func.PCData = append(Func.PCData, uint32(len(linker.pctab)))
		linker.pctab = append(linker.pctab, pcdata...)
	}

	for _, name := range symbol.Func.FuncData {
		if _, ok := linker.symMap[name]; !ok {
			if _, ok := linker.objsymbolMap[name]; ok {
				if _, err = linker.addSymbol(name); err != nil {
					return err
				}
			} else if len(name) == 0 {
				//nothing todo
			} else {
				return errors.New("unknown gcobj:" + name)
			}
		}
		if sym, ok := linker.symMap[name]; ok {
			Func.FuncData = append(Func.FuncData, (uintptr)(sym.Offset))
		} else {
			Func.FuncData = append(Func.FuncData, (uintptr)(0))
		}
	}

	if err = linker.addInlineTree(&_func, symbol); err != nil {
		return err
	}

	grow(&linker.pctab, alignof(len(linker.pctab), PtrSize))
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
				return nil, fmt.Errorf("unresolved external symbol: %s", sym.Name)
			}
		} else if sym.Name == TLSNAME {
			//nothing todo
		} else if sym.Kind == symkind.STEXT {
			symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.codeBase)
			codeModule.Syms[sym.Name] = symbolMap[name]
		} else if strings.HasPrefix(sym.Name, ItabPrefix) {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			}
		} else {
			if _, ok := symPtr[name]; !ok {
				if IsEnableStringMap() && strings.HasPrefix(name, TypeStringPrefix) {
					symbolMap[name] = uintptr(linker.symMap[name].Offset) + stringContainer.addr
				} else {
					symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
				}
			} else {
				symbolMap[name] = symPtr[name]
				if strings.HasPrefix(name, MainPkgPrefix) || strings.HasPrefix(name, TypePrefix) {
					if IsEnableStringMap() && strings.HasPrefix(name, TypeStringPrefix) {
						symbolMap[name] = uintptr(linker.symMap[name].Offset) + stringContainer.addr
					} else {
						symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
					}
				}
			}
		}
	}
	return symbolMap, err
}

func (linker *Linker) addFuncTab(module *moduledata, _func *_func, symbolMap map[string]uintptr) (err error) {
	funcname := gostringnocopy(&linker.funcnametab[_func.nameoff])
	setfuncentry(_func, symbolMap[funcname], module.text)
	Func := linker.symMap[funcname].Func

	if err = stackobject.AddStackObject(funcname, linker.symMap, symbolMap, module.noptrdata); err != nil {
		return err
	}
	if err = linker.addDeferReturn(_func); err != nil {
		return err
	}

	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(_func)), _FuncSize)

	if _func.npcdata > 0 {
		append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&(Func.PCData[0]))), Uint32Size*int(_func.npcdata))
	}

	if _func.nfuncdata > 0 {
		addfuncdata(module, Func, _func)
	}

	return err
}

func (linker *Linker) buildModule(codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	module := codeModule.module
	module.pclntable = append(module.pclntable, linker.functab...)
	module.minpc = uintptr(segment.codeBase)
	module.maxpc = uintptr(segment.codeBase + segment.codeOffset)
	module.types = uintptr(segment.dataBase)
	module.etypes = uintptr(segment.dataBase + segment.dataOffset)
	module.text = uintptr(segment.codeBase)
	module.etext = uintptr(segment.codeBase + len(linker.code))
	module.data = uintptr(segment.dataBase)
	module.edata = uintptr(segment.dataBase) + uintptr(segment.dataLen)
	module.noptrdata = module.edata
	module.enoptrdata = module.noptrdata + uintptr(segment.noptrdataLen)
	module.bss = module.enoptrdata
	module.ebss = module.bss + uintptr(segment.bssLen)
	module.noptrbss = module.ebss
	module.enoptrbss = module.noptrbss + uintptr(segment.noptrbssLen)
	module.end = max(uintptr(segment.codeBase+segment.codeOffset), uintptr(segment.dataBase+segment.dataOffset))

	module.ftab = append(module.ftab, initfunctab(module.minpc, uintptr(len(module.pclntable)), module.text))
	for index, _func := range linker._func {
		funcname := gostringnocopy(&linker.funcnametab[_func.nameoff])
		module.ftab = append(module.ftab, initfunctab(symbolMap[funcname], uintptr(len(module.pclntable)), module.text))
		if err = linker.addFuncTab(module, linker._func[index], symbolMap); err != nil {
			return err
		}
	}
	module.ftab = append(module.ftab, initfunctab(module.maxpc, uintptr(len(module.pclntable)), module.text))

	//see:^src/cmd/link/internal/ld/pcln.go findfunctab
	funcbucket := []findfuncbucket{}
	for k, _func := range linker._func {
		funcname := gostringnocopy(&linker.funcnametab[_func.nameoff])
		x := linker.symMap[funcname].Offset
		b := x / pcbucketsize
		i := x % pcbucketsize / (pcbucketsize / nsub)
		for lb := b - len(funcbucket); lb >= 0; lb-- {
			funcbucket = append(funcbucket, findfuncbucket{
				idx: uint32(k)})
		}
		if funcbucket[b].subbuckets[i] == 0 && b != 0 && i != 0 {
			if k-int(funcbucket[b].idx) >= pcbucketsize/minfunc {
				return fmt.Errorf("over %d func in one funcbuckets", k-int(funcbucket[b].idx))
			}
			funcbucket[b].subbuckets[i] = byte(k - int(funcbucket[b].idx))
		}
	}
	length := len(funcbucket) * FindFuncBucketSize
	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&funcbucket[0])), length)
	module.findfunctab = (uintptr)(unsafe.Pointer(&module.pclntable[len(module.pclntable)-length]))

	if err = linker.addgcdata(codeModule, symbolMap); err != nil {
		return err
	}
	for name, addr := range symbolMap {
		if strings.HasPrefix(name, TypePrefix) &&
			!strings.HasPrefix(name, TypeDoubleDotPrefix) &&
			addr >= module.types && addr < module.etypes {
			module.typelinks = append(module.typelinks, int32(addr-module.types))
			module.typemap[typeOff(addr-module.types)] = (*_type)(unsafe.Pointer(addr))
		}
	}
	initmodule(codeModule.module, linker)

	modulesLock.Lock()
	addModule(codeModule)
	modulesLock.Unlock()
	additabs(codeModule.module)
	moduledataverify1(codeModule.module)
	modulesinit()
	typelinksinit() // Deduplicate typelinks across all modules
	return err
}

func (linker *Linker) deduplicateTypeDescriptors(codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	// Having called addModule and runtime.modulesinit(), we can now safely use typesEqual()
	// (which depended on the module being in the linked list for safe name resolution of types).
	// This means we can now deduplicate type descriptors in the actual code
	// by relocating their addresses to the equivalent *_type in the main module

	// We need to deduplicate type symbols with the main module according to type hash, since type assertion
	// uses *_type pointer equality and many overlapping or builtin types may be included twice
	// We have to do this after adding the module to the linked list since deduplication
	// depends on symbol resolution across all modules
	typehash := make(map[uint32][]*_type, len(firstmoduledata.typelinks))

	firstModule := activeModules()[0]
collect:
	for _, tl := range firstModule.typelinks {
		var t *_type
		if firstModule.typemap == nil {
			t = (*_type)(unsafe.Pointer(firstModule.types + uintptr(tl)))
		} else {
			t = firstModule.typemap[typeOff(tl)]
		}

		// Add to typehash if not seen before.
		tlist := typehash[t.hash]
		for _, tcur := range tlist {
			if tcur == t {
				continue collect
			}
		}
		typehash[t.hash] = append(tlist, t)
	}

	segment := &codeModule.segment
	byteorder := linker.Arch.ByteOrder
	for _, symbol := range linker.symMap {
		for _, loc := range symbol.Reloc {
			addr := symbolMap[loc.Sym.Name]
			relocSym := loc.Sym
			relocByte := segment.dataByte
			addrBase := segment.dataBase
			if symbol.Kind == symkind.STEXT {
				addrBase = segment.codeBase
				relocByte = segment.codeByte
			}

			if addr != InvalidHandleValue && relocSym.Kind == symkind.SRODATA &&
				strings.HasPrefix(relocSym.Name, TypePrefix) &&
				!strings.HasPrefix(relocSym.Name, TypeDoubleDotPrefix) && relocSym.Offset != -1 {

				// if this is pointing to a type descriptor at an offset inside this binary, we should deduplicate it against
				// already known types from other modules to allow fast type assertion using *_type pointer equality
				t := (*_type)(unsafe.Pointer(addr))
				for _, candidate := range typehash[t.hash] {
					seen := map[_typePair]struct{}{}
					if typesEqual(t, candidate, seen) {
						t = candidate
						break
					}
				}

				// Only relocate code if the type is a duplicate
				if uintptr(unsafe.Pointer(t)) != addr {
					addr = uintptr(unsafe.Pointer(t))
					switch loc.Type {
					case reloctype.R_PCREL:
						// The replaced t from another module will probably yield a massive negative offset, but that's ok as
						// PC-relative addressing is allowed to be negative (even if not very cache friendly)
						offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
						if offset > 0x7FFFFFFF || offset < -0x80000000 {
							err = fmt.Errorf("symName: %s offset: %d overflows!\n", relocSym.Name, offset)
						}
						byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
					case reloctype.R_ADDR, reloctype.R_WEAKADDR:
						// TODO - sanity check this
						address := uintptr(int(addr) + loc.Add)
						putAddress(byteorder, relocByte[loc.Offset:], uint64(address))
					case reloctype.R_ADDROFF, reloctype.R_WEAKADDROFF, reloctype.R_METHODOFF:
						if symbol.Kind == symkind.STEXT {
							err = fmt.Errorf("impossible! Sym: %s located on code segment!\n", relocSym.Name)
						}
						// TODO - sanity check this
						offset := int(addr) - segment.codeBase + loc.Add
						if offset > 0x7FFFFFFF || offset < -0x80000000 {
							err = fmt.Errorf("symName: %s offset: %d overflows!\n", relocSym.Name, offset)
						}
						byteorder.PutUint32(segment.dataByte[loc.Offset:], uint32(offset))
					case reloctype.R_USETYPE, reloctype.R_USEIFACE, reloctype.R_USEIFACEMETHOD, reloctype.R_ADDRCUOFF:
						// nothing to do
					default:
						// TODO - should we attempt to rewrite other relocations which point at *_types too?
					}
				}
			}
		}
	}
	return err
}

func (linker *Linker) UnresolvedExternalSymbols(symbolMap map[string]uintptr) map[string]*obj.Sym {
	symMap := make(map[string]*obj.Sym)
	for symName, sym := range linker.symMap {
		if sym.Offset == InvalidOffset {
			if _, ok := symbolMap[symName]; !ok {
				if _, ok := linker.objsymbolMap[symName]; !ok {
					symMap[symName] = sym
				}
			}
		}
	}
	return symMap
}

func Load(linker *Linker, symPtr map[string]uintptr) (codeModule *CodeModule, err error) {
	codeModule = &CodeModule{
		Syms:   make(map[string]uintptr),
		module: &moduledata{typemap: make(map[typeOff]*_type)},
	}
	codeModule.codeLen = len(linker.code)
	codeModule.dataLen = len(linker.data)
	codeModule.noptrdataLen = len(linker.noptrdata)
	codeModule.bssLen = len(linker.bss)
	codeModule.noptrbssLen = len(linker.noptrbss)
	codeModule.sumDataLen = codeModule.dataLen + codeModule.noptrdataLen + codeModule.bssLen + codeModule.noptrbssLen
	codeModule.maxLengthCode = alignof(codeModule.codeLen*4, PageSize)
	codeModule.maxLengthData = alignof(codeModule.sumDataLen*4, PageSize)
	codeByte, err := Mmap(codeModule.maxLengthCode)
	if err != nil {
		return nil, err
	}
	dataByte, err := MmapData(codeModule.maxLengthData)
	if err != nil {
		return nil, err
	}

	codeModule.codeByte = codeByte
	codeModule.dataByte = dataByte
	codeModule.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	codeModule.dataBase = int((*sliceHeader)(unsafe.Pointer(&dataByte)).Data)
	copy(codeModule.codeByte, linker.code)
	codeModule.codeOffset = codeModule.codeLen

	copy(codeModule.dataByte[codeModule.dataOffset:], linker.data)
	codeModule.dataOffset += codeModule.dataLen

	copy(codeModule.dataByte[codeModule.dataOffset:], linker.noptrdata)
	codeModule.dataOffset += codeModule.noptrdataLen

	copy(codeModule.dataByte[codeModule.dataOffset:], linker.bss)
	codeModule.dataOffset += codeModule.bssLen

	copy(codeModule.dataByte[codeModule.dataOffset:], linker.noptrbss)
	codeModule.dataOffset += codeModule.noptrbssLen

	var symbolMap map[string]uintptr
	if symbolMap, err = linker.addSymbolMap(symPtr, codeModule); err == nil {
		if err = linker.relocate(codeModule, symbolMap); err == nil {
			if err = linker.buildModule(codeModule, symbolMap); err == nil {
				if err = linker.deduplicateTypeDescriptors(codeModule, symbolMap); err == nil {
					mmap.MakeThreadJITCodeExecutable(uintptr(codeModule.codeBase), codeModule.maxLengthCode)
					if err = linker.doInitialize(codeModule, symbolMap); err == nil {
						return codeModule, err
					}
				}
			}
		}
	}
	if err != nil {
		err2 := Munmap(codeByte)
		err3 := Munmap(dataByte)
		if err2 != nil || err3 != nil {
			err = fmt.Errorf("failed to munmap (%s|%s) after linker error: %w", err2, err3, err)
		}
	}
	return nil, err
}

func (cm *CodeModule) Unload() error {
	removeitabs(cm.module)
	runtime.GC()
	modulesLock.Lock()
	removeModule(cm.module)
	modulesLock.Unlock()
	modulesinit()
	return Munmap(cm.codeByte)
}
