package goloader

import (
	"cmd/objfile/sys"
	"encoding/binary"
	"errors"
	"fmt"
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
	codeByte     []byte
	codeBase     int
	dataBase     int
	sumDataLen   int
	dataLen      int
	noptrdataLen int
	bssLen       int
	noptrbssLen  int
	codeLen      int
	maxLength    int
	offset       int
}

type Linker struct {
	code         []byte
	data         []byte
	noptrdata    []byte
	bss          []byte
	noptrbss     []byte
	symMap       map[string]*obj.Sym
	objsymbolMap map[string]*obj.ObjSymbol
	namemap      map[string]int
	filetab      []uint32
	pclntable    []byte
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

//initialize Linker
func initLinker() *Linker {
	linker := &Linker{
		symMap:       make(map[string]*obj.Sym),
		objsymbolMap: make(map[string]*obj.ObjSymbol),
		namemap:      make(map[string]int),
	}
	head := make([]byte, unsafe.Sizeof(pcHeader{}))
	copy(head, obj.ModuleHeadx86)
	linker.pclntable = append(linker.pclntable, head...)
	linker.pclntable[len(obj.ModuleHeadx86)-1] = PtrSize
	return linker
}

func (linker *Linker) addSymbols() error {
	//static_tmp is 0, golang compile not allocate memory.
	linker.noptrdata = append(linker.noptrdata, make([]byte, IntSize)...)
	for _, objSym := range linker.objsymbolMap {
		if objSym.Kind == symkind.STEXT && objSym.DupOK == false {
			_, err := linker.addSymbol(objSym.Name)
			if err != nil {
				return err
			}
		}
		if objSym.Kind == symkind.SNOPTRDATA {
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
			if IsEnableStringMap() && strings.HasPrefix(sym.Name, TypeStringPerfix) {
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
		if IsEnableStringMap() && strings.HasPrefix(symbol.Name, TypeStringPerfix) {
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
					return nil, fmt.Errorf("Symbol:%s size:%d>IntSize:%d\n", reloc.Sym.Name, reloc.Size, IntSize)
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
				linker.symMap[reloc.Sym.Name] = reloc.Sym
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
	cuOffset := len(linker.filetab) - 1
	for _, fileName := range symbol.Func.File {
		if offset, ok := linker.namemap[fileName]; !ok {
			linker.filetab = append(linker.filetab, (uint32)(len(linker.pclntable)))
			linker.namemap[fileName] = len(linker.pclntable)
			fileName = strings.TrimPrefix(fileName, FileSymPrefix)
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
	linker.pclntable = append(linker.pclntable, symbol.Func.PCFile...)

	pclnOff := len(linker.pclntable)
	linker.pclntable = append(linker.pclntable, symbol.Func.PCLine...)

	_func := initfunc(symbol, nameOff, pcspOff, pcfileOff, pclnOff, cuOffset)
	linker._func = append(linker._func, &_func)
	Func := linker.symMap[symbol.Name].Func
	for _, pcdata := range symbol.Func.PCData {
		Func.PCData = append(Func.PCData, uint32(len(linker.pclntable)))
		linker.pclntable = append(linker.pclntable, pcdata...)
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

	grow(&linker.pclntable, alignof(len(linker.pclntable), PtrSize))
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
		} else if sym.Kind == symkind.STEXT {
			symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.codeBase)
			codeModule.Syms[sym.Name] = symbolMap[name]
		} else if strings.HasPrefix(sym.Name, ItabPrefix) {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			}
		} else {
			if _, ok := symPtr[name]; !ok {
				if IsEnableStringMap() && strings.HasPrefix(name, TypeStringPerfix) {
					symbolMap[name] = uintptr(linker.symMap[name].Offset) + stringContainer.addr
				} else {
					symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
				}
			} else {
				symbolMap[name] = symPtr[name]
				if strings.HasPrefix(name, MainPkgPrefix) || strings.HasPrefix(name, TypePrefix) {
					if IsEnableStringMap() && strings.HasPrefix(name, TypeStringPerfix) {
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
	funcname := gostringnocopy(&linker.pclntable[_func.nameoff])
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
	module.pclntable = append(module.pclntable, linker.pclntable...)
	module.minpc = uintptr(segment.codeBase)
	module.maxpc = uintptr(segment.codeBase + segment.offset)
	module.types = uintptr(segment.codeBase)
	module.etypes = uintptr(segment.codeBase + segment.offset)
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
	module.end = uintptr(segment.codeBase + segment.offset)

	module.ftab = append(module.ftab, initfunctab(module.minpc, uintptr(len(module.pclntable)), module.text))
	for index, _func := range linker._func {
		funcname := gostringnocopy(&linker.pclntable[_func.nameoff])
		module.ftab = append(module.ftab, initfunctab(symbolMap[funcname], uintptr(len(module.pclntable)), module.text))
		if err = linker.addFuncTab(module, linker._func[index], symbolMap); err != nil {
			return err
		}
	}
	module.ftab = append(module.ftab, initfunctab(module.maxpc, uintptr(len(module.pclntable)), module.text))

	//see:^src/cmd/link/internal/ld/pcln.go findfunctab
	funcbucket := []findfuncbucket{}
	for k, _func := range linker._func {
		funcname := gostringnocopy(&linker.pclntable[_func.nameoff])
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
			module.typemap[typeOff(addr-module.types)] = addr
		}
	}
	initmodule(codeModule.module, linker)

	modulesLock.Lock()
	addModule(codeModule)
	modulesLock.Unlock()
	additabs(codeModule.module)
	moduledataverify1(codeModule.module)
	modulesinit()

	return err
}

func Load(linker *Linker, symPtr map[string]uintptr) (codeModule *CodeModule, err error) {
	codeModule = &CodeModule{
		Syms:   make(map[string]uintptr),
		module: &moduledata{typemap: make(map[typeOff]uintptr)},
	}
	codeModule.codeLen = len(linker.code)
	codeModule.dataLen = len(linker.data)
	codeModule.noptrdataLen = len(linker.noptrdata)
	codeModule.bssLen = len(linker.bss)
	codeModule.noptrbssLen = len(linker.noptrbss)
	codeModule.sumDataLen = codeModule.dataLen + codeModule.noptrdataLen + codeModule.bssLen + codeModule.noptrbssLen
	codeModule.maxLength = alignof((codeModule.codeLen+codeModule.sumDataLen)*2, PageSize)
	codeByte, err := Mmap(codeModule.maxLength)
	if err != nil {
		return nil, err
	}

	codeModule.codeByte = codeByte
	codeModule.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	codeModule.dataBase = codeModule.codeBase + codeModule.codeLen
	copy(codeModule.codeByte, linker.code)
	codeModule.offset = codeModule.codeLen
	copy(codeModule.codeByte[codeModule.offset:], linker.data)
	codeModule.offset += codeModule.dataLen
	copy(codeModule.codeByte[codeModule.offset:], linker.noptrdata)
	codeModule.offset += codeModule.noptrdataLen
	copy(codeModule.codeByte[codeModule.offset:], linker.bss)
	codeModule.offset += codeModule.bssLen
	copy(codeModule.codeByte[codeModule.offset:], linker.noptrbss)
	codeModule.offset += codeModule.noptrbssLen

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
	modulesinit()
	Munmap(cm.codeByte)
}
