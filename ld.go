package goloader

import (
	"cmd/objfile/sys"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/pkujhd/goloader/objabi/reloctype"

	"github.com/pkujhd/goloader/constants"
	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/funcalign"
	"github.com/pkujhd/goloader/objabi/symkind"
	"github.com/pkujhd/goloader/stackobject"
)

// ourself defined struct
// code segment
type codeSeg struct {
	codeByte []byte
	codeBase int
	length   int
	maxLen   int
	codeOff  int
}

// data segment
type dataSeg struct {
	dataByte     []byte
	dataBase     int
	length       int
	maxLen       int
	dataLen      int
	noptrdataLen int
	bssLen       int
	noptrbssLen  int
	dataOff      int
}

type segment struct {
	codeSeg
	dataSeg
}

type gcData struct {
	gcdata []byte
	gcbss  []byte
}

type CodeModule struct {
	segment
	gcData
	Syms      map[string]uintptr
	stringMap map[string]*string
	module    *moduledata
}

type LinkerData struct {
	Code      []byte
	Data      []byte
	Noptrdata []byte
	Bss       []byte
	Noptrbss  []byte
}

type Linker struct {
	LinkerData
	SymMap             map[string]*obj.Sym
	ObjSymbolMap       map[string]*obj.ObjSymbol
	NameMap            map[string]int
	StringMap          map[string]*string
	CgoImportMap       map[string]*obj.CgoImport
	CgoFuncs           map[string]int
	UnImplementedTypes map[string]map[string]int
	Filetab            []uint32
	Pclntable          []byte
	Funcs              []*_func
	Packages           map[string]*obj.Pkg
	Arch               *sys.Arch
	CUOffset           int32
	ExtraData          int
	AdaptedOffset      bool
}

var (
	modules     = make(map[interface{}]bool)
	modulesLock sync.Mutex
)

// initialize Linker
func initLinker() *Linker {
	linker := &Linker{
		SymMap:             make(map[string]*obj.Sym),
		ObjSymbolMap:       make(map[string]*obj.ObjSymbol),
		NameMap:            make(map[string]int),
		StringMap:          make(map[string]*string),
		CgoImportMap:       make(map[string]*obj.CgoImport),
		CgoFuncs:           make(map[string]int),
		UnImplementedTypes: make(map[string]map[string]int),
		Packages:           make(map[string]*obj.Pkg),
		CUOffset:           0,
		ExtraData:          0,
		AdaptedOffset:      false,
	}
	linker.Pclntable = make([]byte, PCHeaderSize)
	return linker
}

func (linker *Linker) initPcHeader() {
	pcheader := (*pcHeader)(unsafe.Pointer(&linker.Pclntable[0]))
	pcheader.magic = magic
	pcheader.minLC = uint8(linker.Arch.MinLC)
	pcheader.ptrSize = uint8(linker.Arch.PtrSize)
}

func (linker *Linker) addFiles(files []string) {
	linker.CUOffset += int32(len(files))
	for _, fileName := range files {
		if offset, ok := linker.NameMap[fileName]; !ok {
			linker.Filetab = append(linker.Filetab, (uint32)(len(linker.Pclntable)))
			linker.NameMap[fileName] = len(linker.Pclntable)
			fileName = strings.TrimPrefix(fileName, FileSymPrefix)
			linker.Pclntable = append(linker.Pclntable, []byte(fileName)...)
			linker.Pclntable = append(linker.Pclntable, ZeroByte)
		} else {
			linker.Filetab = append(linker.Filetab, uint32(offset))
		}
	}
}

func (linker *Linker) addSymbols() error {
	//static_tmp is 0, golang compile not allocate memory.
	linker.Noptrdata = append(linker.Noptrdata, make([]byte, IntSize)...)
	for _, objSym := range linker.ObjSymbolMap {
		if objSym.Kind == symkind.STEXT && objSym.DupOK == false {
			if _, err := linker.addSymbol(objSym.Name, nil); err != nil {
				return err
			}
		}
	}
	for _, pkg := range linker.Packages {
		initFuncName := getInitFuncName(pkg.PkgPath)
		if _, ok := linker.ObjSymbolMap[initFuncName]; ok {
			if _, err := linker.addSymbol(initFuncName, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (linker *Linker) adaptSymbolOffset() {
	if linker.AdaptedOffset == false {
		for _, sym := range linker.SymMap {
			offset := 0
			switch sym.Kind {
			case symkind.SNOPTRDATA, symkind.SRODATA:
				if !strings.HasPrefix(sym.Name, constants.TypeStringPrefix) {
					offset += len(linker.Data)
				}
			case symkind.SBSS:
				offset += len(linker.Data) + len(linker.Noptrdata)
			case symkind.SNOPTRBSS:
				offset += len(linker.Data) + len(linker.Noptrdata) + len(linker.Bss)
			}
			if sym.Offset != InvalidOffset {
				sym.Offset += offset
			}
			if offset != 0 {
				for index := range sym.Reloc {
					if sym.Reloc[index].Offset != InvalidOffset {
						sym.Reloc[index].Offset += offset
					}
				}
			}
		}
		linker.AdaptedOffset = true
	}
}

func (linker *Linker) addSymbol(name string, symPtr map[string]uintptr) (symbol *obj.Sym, err error) {
	if symbol, ok := linker.SymMap[name]; ok && symbol.Offset != InvalidOffset {
		return symbol, nil
	}
	objsym := linker.ObjSymbolMap[name]
	symbol = &obj.Sym{Name: objsym.Name, Kind: objsym.Kind}
	linker.SymMap[symbol.Name] = symbol
	if symPtr != nil {
		if _, ok := symPtr[name]; ok {
			symbol.Offset = InvalidOffset
			return symbol, nil
		}
	}
	switch symbol.Kind {
	case symkind.STEXT:
		symbol.Offset = len(linker.Code)
		linker.Code = append(linker.Code, objsym.Data...)
		if isX86_64(linker.Arch.Name) {
			obj.MarkReloc(objsym.Data, objsym.Reloc, symbol.Offset, linker.Arch.Name)
		}
		expandFunc(linker, objsym, symbol)
		if len(linker.Code)-symbol.Offset < minfunc {
			linker.Code = append(linker.Code, createArchNops(linker.Arch, minfunc-(len(linker.Code)-symbol.Offset))...)
		}
		bytearrayAlignNops(linker.Arch, &linker.Code, funcalign.GetFuncAlign(linker.Arch))
		symbol.Func = &obj.Func{}
		if err := linker.readFuncData(linker.ObjSymbolMap[name], symPtr); err != nil {
			return nil, err
		}
	case symkind.SDATA:
		symbol.Offset = len(linker.Data)
		linker.Data = append(linker.Data, objsym.Data...)
		bytearrayAlign(&linker.Data, PtrSize)
	case symkind.SNOPTRDATA, symkind.SRODATA:
		//because golang string assignment is pointer assignment, so store go.string constants in heap.
		if strings.HasPrefix(symbol.Name, constants.TypeStringPrefix) {
			data := make([]byte, len(objsym.Data))
			copy(data, objsym.Data)
			stringVal := string(data)
			linker.StringMap[symbol.Name] = &stringVal
		} else {
			symbol.Offset = len(linker.Noptrdata)
			linker.Noptrdata = append(linker.Noptrdata, objsym.Data...)
			bytearrayAlign(&linker.Noptrdata, PtrSize)
		}
	case symkind.SBSS:
		symbol.Offset = len(linker.Bss)
		linker.Bss = append(linker.Bss, objsym.Data...)
		bytearrayAlign(&linker.Bss, PtrSize)
	case symkind.SNOPTRBSS:
		symbol.Offset = len(linker.Noptrbss)
		linker.Noptrbss = append(linker.Noptrbss, objsym.Data...)
		bytearrayAlign(&linker.Noptrbss, PtrSize)
	default:
		return nil, fmt.Errorf("invalid symbol:%s kind:%d", symbol.Name, symbol.Kind)
	}

	for _, loc := range objsym.Reloc {
		reloc := loc
		reloc.Offset += symbol.Offset
		if reloc.Epilogue.Offset != 0 {
			reloc.Epilogue.Offset += symbol.Offset
		}
		if _, ok := linker.ObjSymbolMap[reloc.SymName]; ok {
			if symPtr != nil && !isMmapInLowAddress(linker.Arch.Name) &&
				(reloc.Type == reloctype.R_METHODOFF || reloc.Type == reloctype.R_ADDROFF || reloc.Type == reloctype.R_WEAKADDROFF) {
				delete(symPtr, reloc.SymName)
			}
			relocSym, err := linker.addSymbol(reloc.SymName, symPtr)
			if err != nil {
				return nil, err
			}
			if relocSym != nil && len(linker.ObjSymbolMap[reloc.SymName].Data) == 0 && reloc.Size > 0 {
				//static_tmp is 0, golang compile not allocate memory.
				//goloader add IntSize bytes on linker.Noptrdata[0]
				if reloc.Size <= IntSize {
					relocSym.Offset = 0
				} else {
					return nil, fmt.Errorf("Symbol:%s size:%d>IntSize:%d\n", relocSym.Name, reloc.Size, IntSize)
				}
			}
		} else {
			if _, ok := linker.SymMap[reloc.SymName]; !ok && reloc.SymName != EmptyString {
				relocSym := &obj.Sym{Name: reloc.SymName, Offset: InvalidOffset}
				if strings.HasPrefix(reloc.SymName, constants.TypeImportPathPrefix) {
					path := strings.Trim(strings.TrimPrefix(reloc.SymName, constants.TypeImportPathPrefix), ".")
					relocSym.Kind = symkind.SNOPTRDATA
					relocSym.Offset = len(linker.Noptrdata)
					//name memory layout
					//name { tagLen(byte), len(uint16), str*}
					nameLen := []byte{0, 0, 0}
					binary.BigEndian.PutUint16(nameLen[1:], uint16(len(path)))
					linker.Noptrdata = append(linker.Noptrdata, nameLen...)
					linker.Noptrdata = append(linker.Noptrdata, path...)
					linker.Noptrdata = append(linker.Noptrdata, ZeroByte)
					bytearrayAlign(&linker.Noptrbss, PtrSize)
				}
				if ispreprocesssymbol(reloc.SymName) {
					bytes := make([]byte, UInt64Size)
					if err := preprocesssymbol(linker.Arch.ByteOrder, reloc.SymName, bytes); err != nil {
						return nil, err
					} else {
						relocSym.Kind = symkind.SNOPTRDATA
						relocSym.Offset = len(linker.Noptrdata)
						linker.Noptrdata = append(linker.Noptrdata, bytes...)
						bytearrayAlign(&linker.Noptrbss, PtrSize)
					}
				}
				if reloc.Size > 0 {
					linker.SymMap[reloc.SymName] = relocSym
				}
			}
		}
		symbol.Reloc = append(symbol.Reloc, reloc)
	}

	if objsym.Type != EmptyString {
		if _, ok := linker.SymMap[objsym.Type]; !ok {
			if _, ok := linker.ObjSymbolMap[objsym.Type]; !ok {
				linker.SymMap[objsym.Type] = &obj.Sym{Name: objsym.Type, Offset: InvalidOffset}
			} else {
				linker.addSymbol(objsym.Type, symPtr)
			}
		}
	}
	return symbol, nil
}

func (linker *Linker) readFuncData(symbol *obj.ObjSymbol, symPtr map[string]uintptr) (err error) {
	nameOff := len(linker.Pclntable)
	if offset, ok := linker.NameMap[symbol.Name]; !ok {
		linker.NameMap[symbol.Name] = len(linker.Pclntable)
		linker.Pclntable = append(linker.Pclntable, []byte(symbol.Name)...)
		linker.Pclntable = append(linker.Pclntable, ZeroByte)
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
	pcspOff := len(linker.Pclntable)
	linker.Pclntable = append(linker.Pclntable, symbol.Func.PCSP...)

	pcfileOff := len(linker.Pclntable)
	linker.Pclntable = append(linker.Pclntable, symbol.Func.PCFile...)

	pclnOff := len(linker.Pclntable)
	linker.Pclntable = append(linker.Pclntable, symbol.Func.PCLine...)

	_func := initfunc(symbol, nameOff, pcspOff, pcfileOff, pclnOff, int(symbol.Func.CUOffset))
	linker.Funcs = append(linker.Funcs, &_func)
	Func := linker.SymMap[symbol.Name].Func
	for _, pcdata := range symbol.Func.PCData {
		if len(pcdata) == 0 {
			Func.PCData = append(Func.PCData, 0)
		} else {
			Func.PCData = append(Func.PCData, uint32(len(linker.Pclntable)))
			linker.Pclntable = append(linker.Pclntable, pcdata...)
		}
	}

	for _, name := range symbol.Func.FuncData {
		if name == EmptyString {
			Func.FuncData = append(Func.FuncData, (uintptr)(0))
		} else {
			if _, ok := linker.SymMap[name]; !ok {
				if _, ok := linker.ObjSymbolMap[name]; ok {
					if _, err = linker.addSymbol(name, symPtr); err != nil {
						return err
					}
				} else {
					return errors.New("unknown gcobj:" + name)
				}
			}
			if sym, ok := linker.SymMap[name]; ok {
				Func.FuncData = append(Func.FuncData, (uintptr)(sym.Offset))
			} else {
				Func.FuncData = append(Func.FuncData, (uintptr)(0))
			}
		}
	}

	if err = linker.addInlineTree(&_func, symbol); err != nil {
		return err
	}

	grow(&linker.Pclntable, alignof(len(linker.Pclntable), PtrSize))
	return
}

func (linker *Linker) addSymbolMap(symPtr map[string]uintptr, codeModule *CodeModule) (symbolMap map[string]uintptr, err error) {
	symbolMap = make(map[string]uintptr)
	segment := &codeModule.segment
	for name, sym := range linker.SymMap {
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
		} else if sym.Kind == symkind.STEXT {
			symbolMap[name] = uintptr(sym.Offset + segment.codeBase)
			codeModule.Syms[sym.Name] = symbolMap[name]
		} else if strings.HasPrefix(name, constants.TypeStringPrefix) {
			symbolMap[name] = (*stringHeader)(unsafe.Pointer(linker.StringMap[name])).Data
		} else if name == getInitFuncName(DefaultPkgPath) {
			symbolMap[name] = uintptr(sym.Offset + segment.dataBase)
		} else if ispreprocesssymbol(name) {
			symbolMap[name] = uintptr(sym.Offset + segment.dataBase)
		} else if _, ok := symPtr[name]; ok {
			symbolMap[name] = symPtr[name]
		} else {
			symbolMap[name] = uintptr(sym.Offset + segment.dataBase)
		}
		//fill itablinks
		if isItabName(name) {
			codeModule.module.itablinks = append(codeModule.module.itablinks, (*itab)(adduintptr(symbolMap[name], 0)))
		}
	}
	return symbolMap, err
}

func (linker *Linker) addFuncTab(module *moduledata, _func *_func, symbolMap map[string]uintptr) (err error) {
	funcname := getfuncname(_func, module)
	setfuncentry(_func, symbolMap[funcname], module.text)
	Func := linker.SymMap[funcname].Func

	if err = stackobject.AddStackObject(funcname, linker.SymMap, symbolMap, module.noptrdata); err != nil {
		return err
	}
	if err = linker.addDeferReturn(_func, module); err != nil {
		return err
	}

	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(_func)), _FuncSize)

	if _func.Npcdata > 0 {
		append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&(Func.PCData[0]))), Uint32Size*int(_func.Npcdata))
	}

	if _func.Nfuncdata > 0 {
		addfuncdata(module, Func, _func)
	}
	grow(&module.pclntable, alignof(len(module.pclntable), PtrSize))

	return err
}

func (linker *Linker) buildModule(codeModule *CodeModule, symbolMap, symPtr map[string]uintptr) (err error) {
	segment := &codeModule.segment
	module := codeModule.module
	module.pclntable = append(module.pclntable, linker.Pclntable...)
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
	initmodule(codeModule.module, linker)

	grow(&module.pclntable, alignof(len(module.pclntable), PtrSize))
	module.ftab = append(module.ftab, initfunctab(module.minpc, uintptr(len(module.pclntable)), module.text))
	for index, _func := range linker.Funcs {
		funcname := getfuncname(_func, module)
		module.ftab = append(module.ftab, initfunctab(symbolMap[funcname], uintptr(len(module.pclntable)), module.text))
		if err = linker.addFuncTab(module, linker.Funcs[index], symbolMap); err != nil {
			return err
		}
	}
	module.ftab = append(module.ftab, initfunctab(module.maxpc, uintptr(len(module.pclntable)), module.text))

	// see:^src/cmd/link/internal/ld/pcln.go findfunctab
	funcbucket := []findfuncbucket{}
	for k := 0; k < len(linker.Funcs); k++ {
		lEntry := int(getfuncentry(linker.Funcs[k], module.text) - module.text)
		lb := lEntry / pcbucketsize
		li := lEntry % pcbucketsize / (pcbucketsize / nsub)

		entry := int(module.maxpc - module.text)
		if k < len(linker.Funcs)-1 {
			entry = int(getfuncentry(linker.Funcs[k+1], module.text) - module.text)
		}
		b := entry / pcbucketsize
		i := entry % pcbucketsize / (pcbucketsize / nsub)

		for m := b - len(funcbucket); m >= 0; m-- {
			funcbucket = append(funcbucket, findfuncbucket{idx: uint32(k)})
		}
		if lb < b {
			i = nsub - 1
		}
		for n := li + 1; n <= i; n++ {
			if funcbucket[lb].subbuckets[n] == 0 {
				funcbucket[lb].subbuckets[n] = byte(k - int(funcbucket[lb].idx))
			}
		}
	}
	length := len(funcbucket) * FindFuncBucketSize
	append2Slice(&module.pclntable, uintptr(unsafe.Pointer(&funcbucket[0])), length)
	module.findfunctab = (uintptr)(unsafe.Pointer(&module.pclntable[len(module.pclntable)-length]))

	if err = linker.addgcdata(codeModule, symbolMap); err != nil {
		return err
	}
	for name, symbol := range linker.SymMap {
		if isTypeName(name) {
			typeOff := int32(codeModule.dataBase + symbol.Offset - int(module.types))
			module.typelinks = append(module.typelinks, typeOff)
		}
	}

	modulesLock.Lock()
	addModule(codeModule.module)
	modulesLock.Unlock()
	moduledataverify1(codeModule.module)
	modulesinit()
	typelinksinit()
	addFakeItabs(linker.SymMap, symbolMap, symPtr, linker.UnImplementedTypes, codeModule)
	additabs(codeModule.module)

	return err
}

func Load(linker *Linker, symPtr map[string]uintptr) (codeModule *CodeModule, err error) {
	codeModule = &CodeModule{
		Syms:   make(map[string]uintptr),
		module: &moduledata{typemap: nil},
	}

	//init code segment
	codeSeg := &codeModule.segment.codeSeg
	codeSeg.length = len(linker.Code)
	codeSeg.maxLen = alignof(codeSeg.length, PageSize)
	codeByte, err := Mmap(codeSeg.maxLen)
	if err != nil {
		return nil, err
	}
	codeSeg.codeByte = codeByte
	codeSeg.codeBase = int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	copy(codeSeg.codeByte, linker.Code)
	codeSeg.codeOff = codeSeg.length

	//init data segment
	dataSeg := &codeModule.segment.dataSeg
	dataSeg.dataLen = len(linker.Data)
	dataSeg.noptrdataLen = len(linker.Noptrdata)
	dataSeg.bssLen = len(linker.Bss)
	dataSeg.noptrbssLen = len(linker.Noptrbss)
	dataSeg.length = dataSeg.dataLen + dataSeg.noptrdataLen + dataSeg.bssLen + dataSeg.noptrbssLen
	dataSeg.maxLen = alignof(dataSeg.length+linker.ExtraData, PageSize)
	dataSeg.dataOff = 0
	dataByte, err := MmapData(dataSeg.maxLen)
	if err != nil {
		Munmap(dataSeg.dataByte)
		return nil, err
	}
	dataSeg.dataByte = dataByte
	dataSeg.dataBase = int((*sliceHeader)(unsafe.Pointer(&dataByte)).Data)
	copy(dataSeg.dataByte[dataSeg.dataOff:], linker.Data)
	dataSeg.dataOff = dataSeg.dataLen
	copy(dataSeg.dataByte[dataSeg.dataOff:], linker.Noptrdata)
	dataSeg.dataOff += dataSeg.noptrdataLen
	copy(dataSeg.dataByte[dataSeg.dataOff:], linker.Bss)
	dataSeg.dataOff += dataSeg.bssLen
	copy(dataSeg.dataByte[dataSeg.dataOff:], linker.Noptrbss)
	dataSeg.dataOff += dataSeg.noptrbssLen

	codeModule.stringMap = linker.StringMap

	linker.adaptSymbolOffset()

	var symbolMap map[string]uintptr
	if symbolMap, err = linker.addSymbolMap(symPtr, codeModule); err == nil {
		if err = linker.relocate(codeModule, symbolMap, symPtr); err == nil {
			if err = linker.buildModule(codeModule, symbolMap, symPtr); err == nil {
				MakeThreadJITCodeExecutable(uintptr(codeModule.codeBase), codeSeg.maxLen)
				if err = linker.doInitialize(symPtr, symbolMap); err == nil {
					return codeModule, err
				}
			}
		}
	}
	return nil, err
}

func UnresolvedSymbols(linker *Linker, symPtr map[string]uintptr) []string {
	unresolvedSymbols := make([]string, 0)
	for name, sym := range linker.SymMap {
		if sym.Offset == InvalidOffset {
			if _, ok := linker.CgoImportMap[name]; !ok {
				if _, ok := symPtr[sym.Name]; !ok {
					nName := strings.TrimSuffix(name, GOTPCRELSuffix)
					if name != nName {
						if _, ok := symPtr[nName]; !ok {
							unresolvedSymbols = append(unresolvedSymbols, nName)
						}
					} else {
						unresolvedSymbols = append(unresolvedSymbols, name)
					}
				}
			}
		}
	}
	return unresolvedSymbols
}

func CheckUnimplementedInterface(linker *Linker, symPtr map[string]uintptr) map[string]map[string]int {
	for _, sym := range linker.SymMap {
		if isTypeName(sym.Name) && sym.Offset != InvalidOffset {
			typ := (*_type)(unsafe.Pointer(&(linker.Noptrdata[sym.Offset])))
			if typ.Kind() == reflect.Interface {
				for _, typeName := range getUnimplementedInterfaceType(linker.SymMap[sym.Name], symPtr) {
					if _, ok := linker.UnImplementedTypes[typeName]; !ok {
						linker.UnImplementedTypes[typeName] = make(map[string]int, 0)
					}
					linker.UnImplementedTypes[typeName][sym.Name] = 1
				}
			}
		}
	}
	return linker.UnImplementedTypes
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
