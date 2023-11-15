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

	"github.com/pkujhd/goloader/constants"
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
	maxCodeLength int
	maxDataLength int
	codeOff       int
	dataOff       int
}

type Linker struct {
	code         []byte
	data         []byte
	noptrdata    []byte
	bss          []byte
	noptrbss     []byte
	symMap       map[string]*obj.Sym
	objsymbolMap map[string]*obj.ObjSymbol
	nameMap      map[string]int
	stringMap    map[string]*string
	filetab      []uint32
	pclntable    []byte
	_func        []*_func
	initFuncs    []string
	Arch         *sys.Arch
}

type CodeModule struct {
	segment
	Syms      map[string]uintptr
	stringMap map[string]*string
	module    *moduledata
	gcdata    []byte
	gcbss     []byte
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
		nameMap:      make(map[string]int),
		stringMap:    make(map[string]*string),
	}
	head := make([]byte, unsafe.Sizeof(pcHeader{}))
	copy(head, obj.ModuleHeadx86)
	linker.pclntable = append(linker.pclntable, head...)
	linker.pclntable[len(obj.ModuleHeadx86)-1] = PtrSize
	return linker
}

func (linker *Linker) addFiles(files []string) {
	for _, fileName := range files {
		if offset, ok := linker.nameMap[fileName]; !ok {
			linker.filetab = append(linker.filetab, (uint32)(len(linker.pclntable)))
			linker.nameMap[fileName] = len(linker.pclntable)
			fileName = strings.TrimPrefix(fileName, FileSymPrefix)
			linker.pclntable = append(linker.pclntable, []byte(fileName)...)
			linker.pclntable = append(linker.pclntable, ZeroByte)
		} else {
			linker.filetab = append(linker.filetab, uint32(offset))
		}
	}
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
			if strings.HasPrefix(sym.Name, constants.TypeStringPrefix) {
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
		expandFunc(linker, objsym, symbol)
		bytearrayAlignNops(linker.Arch, &linker.code, PtrSize)
		symbol.Func = &obj.Func{}
		if err := linker.readFuncData(linker.objsymbolMap[name], symbol.Offset); err != nil {
			return nil, err
		}
	case symkind.SDATA:
		symbol.Offset = len(linker.data)
		linker.data = append(linker.data, objsym.Data...)
		bytearrayAlign(&linker.data, PtrSize)
	case symkind.SNOPTRDATA, symkind.SRODATA:
		//because golang string assignment is pointer assignment, so store go.string constants in heap.
		if strings.HasPrefix(symbol.Name, constants.TypeStringPrefix) {
			data := make([]byte, len(objsym.Data))
			copy(data, objsym.Data)
			stringVal := string(data)
			linker.stringMap[symbol.Name] = &stringVal
		} else {
			symbol.Offset = len(linker.noptrdata)
			linker.noptrdata = append(linker.noptrdata, objsym.Data...)
			bytearrayAlign(&linker.noptrdata, PtrSize)
		}
	case symkind.SBSS:
		symbol.Offset = len(linker.bss)
		linker.bss = append(linker.bss, objsym.Data...)
		bytearrayAlign(&linker.bss, PtrSize)
	case symkind.SNOPTRBSS:
		symbol.Offset = len(linker.noptrbss)
		linker.noptrbss = append(linker.noptrbss, objsym.Data...)
		bytearrayAlign(&linker.noptrbss, PtrSize)
	default:
		return nil, fmt.Errorf("invalid symbol:%s kind:%d", symbol.Name, symbol.Kind)
	}

	for _, loc := range objsym.Reloc {
		reloc := loc
		reloc.Offset += symbol.Offset
		if reloc.Epilogue.Offset != 0 {
			reloc.Epilogue.Offset += symbol.Offset
		}
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
			if strings.HasPrefix(reloc.Sym.Name, constants.TypeImportPathPrefix) {
				if exist {
					reloc.Sym = linker.symMap[reloc.Sym.Name]
				} else {
					path := strings.Trim(strings.TrimPrefix(reloc.Sym.Name, constants.TypeImportPathPrefix), ".")
					reloc.Sym.Kind = symkind.SNOPTRDATA
					reloc.Sym.Offset = len(linker.noptrdata)
					//name memory layout
					//name { tagLen(byte), len(uint16), str*}
					nameLen := []byte{0, 0, 0}
					binary.BigEndian.PutUint16(nameLen[1:], uint16(len(path)))
					linker.noptrdata = append(linker.noptrdata, nameLen...)
					linker.noptrdata = append(linker.noptrdata, path...)
					linker.noptrdata = append(linker.noptrdata, ZeroByte)
					bytearrayAlign(&linker.noptrbss, PtrSize)
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
						bytearrayAlign(&linker.noptrbss, PtrSize)
					}
				}
			}
			if !exist && loc.Size > 0 {
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
	nameOff := len(linker.pclntable)
	if offset, ok := linker.nameMap[symbol.Name]; !ok {
		linker.nameMap[symbol.Name] = len(linker.pclntable)
		linker.pclntable = append(linker.pclntable, []byte(symbol.Name)...)
		linker.pclntable = append(linker.pclntable, ZeroByte)
	} else {
		nameOff = offset
	}

	adaptePCFile(linker, symbol)
	for _, reloc := range symbol.Reloc {
		if reloc.Epilogue.Size > 0 {
			patchPCValues(linker, &symbol.Func.PCSP, reloc)
			patchPCValues(linker, &symbol.Func.PCFile, reloc)
			patchPCValues(linker, &symbol.Func.PCLine, reloc)
			for i := range symbol.Func.PCData {
				patchPCValues(linker, &symbol.Func.PCData[i], reloc)
			}
		}
	}
	pcspOff := len(linker.pclntable)
	linker.pclntable = append(linker.pclntable, symbol.Func.PCSP...)

	pcfileOff := len(linker.pclntable)
	linker.pclntable = append(linker.pclntable, symbol.Func.PCFile...)

	pclnOff := len(linker.pclntable)
	linker.pclntable = append(linker.pclntable, symbol.Func.PCLine...)

	_func := initfunc(symbol, nameOff, pcspOff, pcfileOff, pclnOff, int(symbol.Func.CUOffset))
	linker._func = append(linker._func, &_func)
	Func := linker.symMap[symbol.Name].Func
	for _, pcdata := range symbol.Func.PCData {
		if len(pcdata) == 0 {
			Func.PCData = append(Func.PCData, 0)
		} else {
			Func.PCData = append(Func.PCData, uint32(len(linker.pclntable)))
			linker.pclntable = append(linker.pclntable, pcdata...)
		}
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
			} else if addr, ok := symPtr[strings.TrimSuffix(name, GOTPCRELSuffix)]; ok && strings.HasSuffix(name, GOTPCRELSuffix) {
				symbolMap[name] = uintptr(segment.dataBase) + uintptr(segment.dataOff)
				putAddressAddOffset(linker.Arch.ByteOrder, segment.dataByte, &segment.dataOff, uint64(addr))
			} else {
				symbolMap[name] = InvalidHandleValue
				return nil, fmt.Errorf("unresolve external:%s", sym.Name)
			}
		} else if sym.Name == TLSNAME {
			//nothing todo
		} else if sym.Kind == symkind.STEXT {
			symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.codeBase)
			codeModule.Syms[sym.Name] = symbolMap[name]
		} else if strings.HasPrefix(sym.Name, constants.ItabPrefix) {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			}
		} else {
			if strings.HasPrefix(name, constants.TypeStringPrefix) {
				symbolMap[name] = (*stringHeader)(unsafe.Pointer(linker.stringMap[name])).Data
			} else if strings.HasPrefix(name, constants.TypePrefix) {
				if _, ok := linker.symMap[name]; ok {
					symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
				} else {
					symbolMap[name] = symPtr[name]
				}
			} else if _, ok := symPtr[name]; ok {
				symbolMap[name] = symPtr[name]
			} else {
				symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
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
	module.maxpc = uintptr(segment.codeBase + segment.codeOff)
	module.text = uintptr(segment.codeBase)
	module.etext = module.maxpc
	module.data = uintptr(segment.dataBase)
	module.edata = uintptr(segment.dataBase) + uintptr(segment.dataLen)
	module.noptrdata = module.edata
	module.enoptrdata = module.noptrdata + uintptr(segment.noptrdataLen)
	module.bss = module.enoptrdata
	module.ebss = module.bss + uintptr(segment.bssLen)
	module.noptrbss = module.ebss
	module.enoptrbss = module.noptrbss + uintptr(segment.noptrbssLen)
	module.end = module.enoptrbss
	module.types = module.data
	module.etypes = module.enoptrbss

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
		if strings.HasPrefix(name, constants.TypePrefix) &&
			!strings.HasPrefix(name, constants.TypeDoubleDotPrefix) &&
			addr >= module.types && addr < module.etypes {
			module.typelinks = append(module.typelinks, int32(addr-module.types))
		}
	}
	initmodule(codeModule.module, linker)

	modulesLock.Lock()
	addModule(codeModule.module)
	modulesLock.Unlock()
	moduledataverify1(codeModule.module)
	modulesinit()
	typelinksinit()
	additabs(codeModule.module)

	return err
}

func Load(linker *Linker, symPtr map[string]uintptr) (codeModule *CodeModule, err error) {
	codeModule = &CodeModule{
		Syms:   make(map[string]uintptr),
		module: &moduledata{typemap: nil},
	}
	codeModule.codeLen = len(linker.code)
	codeModule.dataLen = len(linker.data)
	codeModule.noptrdataLen = len(linker.noptrdata)
	codeModule.bssLen = len(linker.bss)
	codeModule.noptrbssLen = len(linker.noptrbss)
	codeModule.sumDataLen = codeModule.dataLen + codeModule.noptrdataLen + codeModule.bssLen + codeModule.noptrbssLen
	codeModule.maxCodeLength = alignof((codeModule.codeLen)*2, PageSize)
	codeModule.maxDataLength = alignof((codeModule.sumDataLen)*2, PageSize)
	codeByte, err := Mmap(codeModule.maxCodeLength)
	if err != nil {
		return nil, err
	}
	dataByte, err := MmapData(codeModule.maxDataLength)
	if err != nil {
		return nil, err
	}

	codeModule.codeByte = codeByte
	codeModule.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	copy(codeModule.codeByte, linker.code)
	codeModule.codeOff = codeModule.codeLen

	codeModule.dataByte = dataByte
	codeModule.dataBase = int((*sliceHeader)(unsafe.Pointer(&dataByte)).Data)
	copy(codeModule.dataByte[codeModule.dataOff:], linker.data)
	codeModule.dataOff = codeModule.dataLen
	copy(codeModule.dataByte[codeModule.dataOff:], linker.noptrdata)
	codeModule.dataOff += codeModule.noptrdataLen
	copy(codeModule.dataByte[codeModule.dataOff:], linker.bss)
	codeModule.dataOff += codeModule.bssLen
	copy(codeModule.dataByte[codeModule.dataOff:], linker.noptrbss)
	codeModule.dataOff += codeModule.noptrbssLen

	codeModule.stringMap = linker.stringMap

	var symbolMap map[string]uintptr
	if symbolMap, err = linker.addSymbolMap(symPtr, codeModule); err == nil {
		if err = linker.relocate(codeModule, symbolMap, symPtr); err == nil {
			if err = linker.buildModule(codeModule, symbolMap); err == nil {
				MakeThreadJITCodeExecutable(uintptr(codeModule.codeBase), codeModule.maxCodeLength)
				if err = linker.doInitialize(symPtr, symbolMap); err == nil {
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
	Munmap(cm.dataByte)
}
