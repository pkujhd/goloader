package goloader

import (
	"bytes"
	"cmd/objfile/objabi"
	"cmd/objfile/sys"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/eh-steve/goloader/obj"
	"github.com/eh-steve/goloader/objabi/reloctype"
	"github.com/eh-steve/goloader/objabi/symkind"
	"github.com/eh-steve/goloader/stackobject"
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
	code                   []byte
	data                   []byte
	noptrdata              []byte
	bss                    []byte
	noptrbss               []byte
	cuFiles                []obj.CompilationUnitFiles
	symMap                 map[string]*obj.Sym
	objsymbolMap           map[string]*obj.ObjSymbol
	namemap                map[string]int
	fileNameMap            map[string]int
	cutab                  []uint32
	filetab                []byte
	funcnametab            []byte
	functab                []byte
	pctab                  []byte
	_func                  []*_func
	initFuncs              []string
	symNameOrder           []string
	Arch                   *sys.Arch
	options                LinkerOptions
	heapStringMap          map[string]*string
	appliedADRPRelocs      map[*byte][]byte
	appliedPCRelRelocs     map[*byte][]byte
	pkgNamesWithUnresolved map[string]struct{}
	reachableTypes         map[string]struct{}
	pkgs                   []*obj.Pkg
}

type CodeModule struct {
	segment
	SymbolsByPkg          map[string]map[string]interface{}
	Syms                  map[string]uintptr
	module                *moduledata
	gcdata                []byte
	gcbss                 []byte
	patchedTypeMethodsIfn map[*_type]map[int]struct{}
	patchedTypeMethodsTfn map[*_type]map[int]struct{}
	deduplicatedTypes     map[string]uintptr
	heapStrings           map[string]*string
}

var (
	modules     = make(map[*CodeModule]bool)
	modulesLock sync.Mutex
)

// initialize Linker
func initLinker(opts []LinkerOptFunc) (*Linker, error) {

	linker := &Linker{
		// Pad these tabs out so offsets don't start at 0, which is often used in runtime as a special value for "missing"
		// e.g. runtime/traceback.go and runtime/symtab.go both contain checks like:
		// if f.pcsp == 0 ...
		// and
		// if f.nameoff == 0
		funcnametab:            make([]byte, PtrSize),
		pctab:                  make([]byte, PtrSize),
		symMap:                 make(map[string]*obj.Sym),
		objsymbolMap:           make(map[string]*obj.ObjSymbol),
		namemap:                make(map[string]int),
		fileNameMap:            make(map[string]int),
		heapStringMap:          make(map[string]*string),
		appliedADRPRelocs:      make(map[*byte][]byte),
		appliedPCRelRelocs:     make(map[*byte][]byte),
		pkgNamesWithUnresolved: make(map[string]struct{}),
		reachableTypes:         make(map[string]struct{}),
	}
	linker.Opts(opts...)

	head := make([]byte, unsafe.Sizeof(pcHeader{}))
	copy(head, obj.ModuleHeadx86)
	linker.functab = append(linker.functab, head...)
	linker.functab[len(obj.ModuleHeadx86)-1] = PtrSize
	return linker, nil
}

func (linker *Linker) Opts(linkerOpts ...LinkerOptFunc) {
	for _, opt := range linkerOpts {
		opt(&linker.options)
	}
}

func (linker *Linker) addSymbols(symbolNames []string) error {
	// static_tmp is 0, golang compile not allocate memory.
	linker.noptrdata = append(linker.noptrdata, make([]byte, IntSize)...)

	for _, cuFileSet := range linker.cuFiles {
		for _, fileName := range cuFileSet.Files {
			if offset, ok := linker.fileNameMap[fileName]; !ok {
				linker.cutab = append(linker.cutab, (uint32)(len(linker.filetab)))
				linker.fileNameMap[fileName] = len(linker.filetab)
				fileName = expandGoroot(strings.TrimPrefix(fileName, FileSymPrefix))
				linker.filetab = append(linker.filetab, []byte(fileName)...)
				linker.filetab = append(linker.filetab, ZeroByte)
			} else {
				linker.cutab = append(linker.cutab, uint32(offset))
			}
		}
	}

	for _, objSymName := range symbolNames {
		objSym := linker.objsymbolMap[objSymName]
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
		switch objSym.Kind {
		case symkind.SNOPTRDATA, symkind.SRODATA, symkind.SDATA, symkind.SBSS, symkind.SNOPTRBSS:
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
			if strings.HasPrefix(sym.Name, TypeStringPrefix) {
				// nothing todo
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
				if sym.Reloc[index].EpilogueOffset > 0 {
					sym.Reloc[index].EpilogueOffset += offset
				}
			}
		}
	}
	linker.symNameOrder = symbolNames
	return nil
}

func (linker *Linker) SymbolOrder() []string {
	return linker.symNameOrder
}

func (linker *Linker) addSymbol(name string) (symbol *obj.Sym, err error) {
	if symbol, ok := linker.symMap[name]; ok {
		return symbol, nil
	}
	objsym := linker.objsymbolMap[name]
	symbol = &obj.Sym{Name: objsym.Name, Kind: objsym.Kind, Pkg: objsym.Pkg}
	linker.symMap[symbol.Name] = symbol

	switch symbol.Kind {
	case symkind.STEXT:
		symbol.Offset = len(linker.code)
		linker.code = append(linker.code, objsym.Data...)
		bytearrayAlign(&linker.code, PtrSize)
		for i, reloc := range objsym.Reloc {
			// Pessimistically pad the function text with extra bytes for any relocations which might add extra
			// instructions at the end in the case of a 32 bit overflow. These epilogue PCs need to be added to
			// the PCData, PCLine, PCFile, PCSP etc in case of pre-emption or stack unwinding while the PC is running these hacked instructions.
			// We find the relevant PCValues for the offset of the reloc, and reuse the values for the reloc's epilogue

			if linker.options.NoRelocationEpilogues && !strings.HasPrefix(reloc.Sym.Name, TypeStringPrefix) {
				continue
			}
			switch reloc.Type {
			case reloctype.R_ADDRARM64:
				objsym.Reloc[i].EpilogueOffset = len(linker.code) - symbol.Offset
				objsym.Reloc[i].EpilogueSize = maxExtraInstructionBytesADRP
				linker.code = append(linker.code, make([]byte, maxExtraInstructionBytesADRP)...)
			case reloctype.R_ARM64_PCREL_LDST8, reloctype.R_ARM64_PCREL_LDST16, reloctype.R_ARM64_PCREL_LDST32, reloctype.R_ARM64_PCREL_LDST64:
				objsym.Reloc[i].EpilogueOffset = len(linker.code) - symbol.Offset
				objsym.Reloc[i].EpilogueSize = maxExtraInstructionBytesADRPLDST
				linker.code = append(linker.code, make([]byte, maxExtraInstructionBytesADRPLDST)...)
			case reloctype.R_CALLARM64:
				objsym.Reloc[i].EpilogueOffset = alignof(len(linker.code)-symbol.Offset, PtrSize)
				objsym.Reloc[i].EpilogueSize = maxExtraInstructionBytesCALLARM64
				alignment := alignof(len(linker.code)-symbol.Offset, PtrSize) - (len(linker.code) - symbol.Offset)
				linker.code = append(linker.code, make([]byte, maxExtraInstructionBytesCALLARM64+alignment)...)
			case reloctype.R_PCREL:
				objsym.Reloc[i].EpilogueOffset = len(linker.code) - symbol.Offset
				instructionBytes := objsym.Data[reloc.Offset-2 : reloc.Offset+reloc.Size]
				shortJmp := (objsym.Reloc[i].EpilogueOffset - (reloc.Offset + reloc.Size)) < 128
				opcode := instructionBytes[0]
				var epilogueSize int
				switch opcode {
				case x86amd64LEAcode:
					epilogueSize = maxExtraInstructionBytesPCRELxLEAQ
				case x86amd64MOVcode:
					if shortJmp {
						epilogueSize = maxExtraInstructionBytesPCRELxMOVShort
					} else {
						epilogueSize = maxExtraInstructionBytesPCRELxMOVNear
					}
				case x86amd64CMPLcode:
					if shortJmp {
						epilogueSize = maxExtraInstructionBytesPCRELxCMPLShort
					} else {
						epilogueSize = maxExtraInstructionBytesPCRELxCMPLNear
					}
				default:
					switch instructionBytes[1] {
					case x86amd64CALLcode:
						if shortJmp {
							epilogueSize = maxExtraInstructionBytesPCRELxCALLShort
						} else {
							epilogueSize = maxExtraInstructionBytesPCRELxCALLNear
						}
					case x86amd64JMPcode:
						epilogueSize = maxExtraInstructionBytesPCRELxJMP
					}
				}
				objsym.Reloc[i].EpilogueSize = epilogueSize
				linker.code = append(linker.code, make([]byte, epilogueSize)...)
			case reloctype.R_CALL:
				objsym.Reloc[i].EpilogueOffset = len(linker.code) - symbol.Offset
				objsym.Reloc[i].EpilogueSize = maxExtraInstructionBytesCALL
				linker.code = append(linker.code, make([]byte, maxExtraInstructionBytesCALL)...)
			}
			bytearrayAlign(&linker.code, PtrSize)
		}

		symbol.Func = &obj.Func{}
		if err := linker.readFuncData(linker.objsymbolMap[name], symbol.Offset); err != nil {
			return nil, err
		}
	case symkind.SDATA:
		symbol.Offset = len(linker.data)
		linker.data = append(linker.data, objsym.Data...)
		bytearrayAlign(&linker.data, PtrSize)
	case symkind.SNOPTRDATA, symkind.SRODATA:
		// because golang string assignment is pointer assignment, so store go.string constants
		// in a separate segment and not unload when module unload.
		if strings.HasPrefix(symbol.Name, TypeStringPrefix) {
			data := make([]byte, len(objsym.Data))
			copy(data, objsym.Data)
			stringVal := string(data)
			linker.heapStringMap[symbol.Name] = &stringVal
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
		reloc.Offset = reloc.Offset + symbol.Offset
		reloc.EpilogueOffset = reloc.EpilogueOffset + symbol.Offset
		if _, ok := linker.objsymbolMap[reloc.Sym.Name]; ok {
			reloc.Sym, err = linker.addSymbol(reloc.Sym.Name)
			if err != nil {
				return nil, err
			}
			if len(linker.objsymbolMap[reloc.Sym.Name].Data) == 0 && reloc.Size > 0 {
				// static_tmp is 0, golang compile not allocate memory.
				// goloader add IntSize bytes on linker.noptrdata[0]
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
					// name memory layout
					// name { tagLen(byte), len(uint16), str*}
					nameLen := []byte{0, 0, 0}
					binary.PutUvarint(nameLen[1:], uint64(len(path)))
					linker.noptrdata = append(linker.noptrdata, nameLen...)
					linker.noptrdata = append(linker.noptrdata, path...)
					linker.noptrdata = append(linker.noptrdata, ZeroByte)
					bytearrayAlign(&linker.noptrdata, PtrSize)
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
						bytearrayAlign(&linker.noptrdata, PtrSize)
					}
				}
			}
			if !exist {
				// golang1.8, some function generates more than one (MOVQ (TLS), CX)
				// so when same name symbol in linker.symMap, do not update it
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
				linker.symMap[objsym.Type] = &obj.Sym{Name: objsym.Type, Offset: InvalidOffset, Pkg: objsym.Pkg}
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

	for _, reloc := range symbol.Reloc {
		if reloc.EpilogueOffset > 0 {
			if len(symbol.Func.PCSP) > 0 {
				linker.patchPCValuesForReloc(&symbol.Func.PCSP, reloc.Offset, reloc.EpilogueOffset, reloc.EpilogueSize)
			}
			if len(symbol.Func.PCFile) > 0 {
				linker.patchPCValuesForReloc(&symbol.Func.PCFile, reloc.Offset, reloc.EpilogueOffset, reloc.EpilogueSize)
			}
			if len(symbol.Func.PCLine) > 0 {
				linker.patchPCValuesForReloc(&symbol.Func.PCLine, reloc.Offset, reloc.EpilogueOffset, reloc.EpilogueSize)
			}
			for i, pcdata := range symbol.Func.PCData {
				if len(pcdata) > 0 {
					linker.patchPCValuesForReloc(&symbol.Func.PCData[i], reloc.Offset, reloc.EpilogueOffset, reloc.EpilogueSize)
				}
			}
		}
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
				// nothing todo
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
				// Mark the symbol as a duplicate
				symbolMap[FirstModulePrefix+name] = ptr
			} else {
				symbolMap[name] = InvalidHandleValue
				return nil, fmt.Errorf("unresolved external symbol: %s", sym.Name)
			}
		} else if sym.Name == TLSNAME {
			// nothing todo
		} else if sym.Kind == symkind.STEXT {
			symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.codeBase)
			codeModule.Syms[sym.Name] = symbolMap[name]
			if _, ok := symPtr[name]; ok {
				// Mark the symbol as a duplicate
				symbolMap[FirstModulePrefix+name] = symbolMap[name]
			}
		} else if strings.HasPrefix(sym.Name, ItabPrefix) {
			if ptr, ok := symPtr[sym.Name]; ok {
				symbolMap[name] = ptr
			}
		} else {
			if _, ok := symPtr[name]; !ok {
				if strings.HasPrefix(name, TypeStringPrefix) {
					strPtr := linker.heapStringMap[name]
					if strPtr == nil {
						return nil, fmt.Errorf("impossible! got a nil string for symbol %s", name)
					}
					if len(*strPtr) == 0 {
						// Any address will do, the length is 0, so it should never be read
						symbolMap[name] = uintptr(unsafe.Pointer(linker))
					} else {
						x := (*reflect.StringHeader)(unsafe.Pointer(strPtr))
						symbolMap[name] = x.Data
					}
				} else {
					symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
					if strings.HasSuffix(sym.Name, "·f") {
						codeModule.Syms[sym.Name] = symbolMap[name]
					}
					if strings.HasPrefix(name, TypePrefix) {
						if variant, ok := symbolIsVariant(name); ok && symPtr[variant] != 0 {
							symbolMap[FirstModulePrefix+name] = symPtr[variant]
						}
					}
				}
			} else {
				if strings.HasPrefix(name, MainPkgPrefix) || strings.HasPrefix(name, TypePrefix) {
					symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
					// Record the presence of a duplicate symbol by adding a prefix
					symbolMap[FirstModulePrefix+name] = symPtr[name]
				} else {
					shouldSkipDedup := false
					for _, pkgPath := range linker.options.SkipTypeDeduplicationForPackages {
						if strings.HasPrefix(name, pkgPath) {
							shouldSkipDedup = true
						}
					}
					if shouldSkipDedup {
						// Use the new version of the symbol
						symbolMap[name] = uintptr(linker.symMap[name].Offset + segment.dataBase)
					} else {
						symbolMap[name] = symPtr[name]
						// Mark the symbol as a duplicate
						symbolMap[FirstModulePrefix+name] = symPtr[name]
					}
				}
			}
		}
	}
	if tlsG, ok := symPtr[TLSNAME]; ok {
		symbolMap[TLSNAME] = tlsG
	}
	codeModule.heapStrings = linker.heapStringMap
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

func readPCData(p []byte, startPC uintptr) (pcs []uintptr, vals []int32) {
	pc := startPC
	val := int32(-1)
	if len(p) == 0 {
		return nil, nil
	}
	for {
		var ok bool
		p, ok = step(p, &pc, &val, pc == startPC)
		if !ok {
			break
		}
		pcs = append(pcs, pc)
		vals = append(vals, val)
		if len(p) == 0 {
			break
		}
	}
	return
}

func formatPCData(p []byte, startPC uintptr) string {
	pcs, vals := readPCData(p, startPC)
	var result string
	if len(pcs) == 0 {
		return "()"
	}
	prevPC := startPC
	for i := range pcs {
		result += fmt.Sprintf("(%d-%d => %d), ", prevPC, pcs[i], vals[i])
		prevPC = startPC + pcs[i]
	}
	return result
}

func pcValue(p []byte, targetOffset uintptr) (int32, uintptr) {
	startPC := uintptr(0)
	pc := uintptr(0)
	val := int32(-1)
	if len(p) == 0 {
		return -1, 1<<64 - 1
	}
	prevpc := pc
	for {
		var ok bool
		p, ok = step(p, &pc, &val, pc == startPC)
		if !ok {
			break
		}
		if len(p) == 0 {
			break
		}
		if targetOffset < pc {
			return val, prevpc
		}
		prevpc = pc
	}
	return -1, 1<<64 - 1
}

func (linker *Linker) patchPCValuesForReloc(pcvalues *[]byte, relocOffet int, epilogueOffset int, epilogueSize int) {
	// Use the pcvalue at the offset of the reloc for the entire of that reloc's epilogue.
	// This ensures that if the code is pre-empted or the stack unwound while we're inside the epilogue, the runtime behaves correctly

	var pcQuantum uintptr = 1
	if linker.Arch.Family == sys.ARM64 {
		pcQuantum = 4
	}
	p := *pcvalues
	if len(p) == 0 {
		panic("trying to patch a zero sized pcvalue table. This shouldn't be possible...")
	}
	valAtRelocSite, startPC := pcValue(p, uintptr(relocOffet))
	if startPC == 1<<64-1 && valAtRelocSite == -1 {
		panic(fmt.Sprintf("couldn't interpret pcvalue data when trying to patch it... relocOffset: %d, pcdata: %v\n %s", relocOffet, p, formatPCData(p, 0)))
	}
	if p[len(p)-1] != 0 {
		panic(fmt.Sprintf("got a pcvalue table with an unexpected ending (%d)...\n%s ", p[len(p)-1], formatPCData(p, 0)))
	}
	p = p[:len(p)-1] // Remove the terminating 0

	// Table is (value, PC), (value, PC), (value, PC)... etc
	// Each value is delta encoded (signed) relative to the last, and each PC is delta encoded (unsigned)

	pcs, vals := readPCData(p, 0)
	lastValue := vals[len(vals)-1]
	lastPC := pcs[len(pcs)-1]
	if lastValue == valAtRelocSite {
		// Extend the lastPC delta to absorb our epilogue, keep the value the same
		var pcDelta uintptr
		if len(pcs) > 1 {
			pcDelta = (lastPC - pcs[len(pcs)-2]) / pcQuantum
		} else {
			pcDelta = lastPC / pcQuantum
		}

		buf := make([]byte, 10)
		n := binary.PutUvarint(buf, uint64(pcDelta))
		buf = buf[:n]
		index := bytes.LastIndex(p, buf)
		if index == -1 {
			panic(fmt.Sprintf("could not find varint PC delta of %d (%v)", pcDelta, buf))
		}
		p = p[:index]
		if len(pcs) > 1 {
			pcDelta = (uintptr(epilogueOffset+epilogueSize) - pcs[len(pcs)-2]) / pcQuantum
		} else {
			pcDelta = (uintptr(epilogueOffset + epilogueSize)) / pcQuantum
		}

		buf = make([]byte, 10)
		n = binary.PutUvarint(buf, uint64(pcDelta))
		p = append(p, buf[:n]...)
	} else {
		// Append a new (value, PC) pair
		pcDelta := (epilogueOffset + epilogueSize - int(lastPC)) / int(pcQuantum)
		if pcDelta < 0 {
			panic(fmt.Sprintf("somehow the epilogue is not at the end?? lastPC %d, epilogue offset %d", lastPC, epilogueOffset))
		}
		valDelta := valAtRelocSite - lastValue

		buf := make([]byte, 10)
		n := binary.PutVarint(buf, int64(valDelta))
		p = append(p, buf[:n]...)

		n = binary.PutUvarint(buf, uint64(pcDelta))
		p = append(p, buf[:n]...)
	}

	// Re-add the terminating 0 we stripped off
	p = append(p, 0)

	*pcvalues = p
}

func (linker *Linker) buildModule(codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	module := codeModule.module
	module.pclntable = append(module.pclntable, linker.functab...)
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
		funcname := gostringnocopy(&linker.funcnametab[_func.nameoff])
		module.ftab = append(module.ftab, initfunctab(symbolMap[funcname], uintptr(len(module.pclntable)), module.text))
		if err = linker.addFuncTab(module, linker._func[index], symbolMap); err != nil {
			return err
		}
	}
	module.ftab = append(module.ftab, initfunctab(module.maxpc, uintptr(len(module.pclntable)), module.text))

	// see:^src/cmd/link/internal/ld/pcln.go findfunctab
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
	buildModuleTypeHash(activeModules()[0], typehash)

	patchedTypeMethodsIfn := make(map[*_type]map[int]struct{})
	patchedTypeMethodsTfn := make(map[*_type]map[int]struct{})
	segment := &codeModule.segment
	byteorder := linker.Arch.ByteOrder
	dedupedTypes := map[string]uintptr{}
	for _, symbol := range linker.symMap {
	relocLoop:
		for _, loc := range symbol.Reloc {
			addr := symbolMap[loc.Sym.Name]
			sym := loc.Sym
			relocByte := segment.dataByte
			addrBase := segment.dataBase
			if symbol.Kind == symkind.STEXT {
				addrBase = segment.codeBase
				relocByte = segment.codeByte
			}
			if addr != InvalidHandleValue && sym.Kind == symkind.SRODATA &&
				strings.HasPrefix(sym.Name, TypePrefix) &&
				!strings.HasPrefix(sym.Name, TypeDoubleDotPrefix) && sym.Offset != -1 {

				// if this is pointing to a type descriptor at an offset inside this binary, we should deduplicate it against
				// already known types from other modules to allow fast type assertion using *_type pointer equality
				t := (*_type)(unsafe.Pointer(addr))
				prevT := (*_type)(unsafe.Pointer(addr))
				for _, candidate := range typehash[t.hash] {
					seen := map[_typePair]struct{}{}
					if typesEqual(t, candidate, seen) {
						t = candidate
						break
					}
				}

				// Only relocate code if the type is a duplicate
				if t != prevT {
					_, isVariant := symbolIsVariant(loc.Sym.Name)
					if uintptr(unsafe.Pointer(t)) != symbolMap[FirstModulePrefix+loc.Sym.Name] && !isVariant {
						// This shouldn't be possible and indicates a registration bug
						panic(fmt.Sprintf("found another firstmodule type that wasn't registered by goloader: %s", loc.Sym.Name))
					}
					// Store this for later so we know which types were deduplicated
					dedupedTypes[loc.Sym.Name] = uintptr(unsafe.Pointer(t))

					for _, pkgPathToSkip := range linker.options.SkipTypeDeduplicationForPackages {
						if t.PkgPath() == pkgPathToSkip {
							continue relocLoop
						}
					}
					u := t.uncommon()
					prevU := prevT.uncommon()
					err2 := codeModule.patchTypeMethodOffsets(t, u, prevU, patchedTypeMethodsIfn, patchedTypeMethodsTfn)
					if err2 != nil {
						return err2
					}

					addr = uintptr(unsafe.Pointer(t))
					if linker.options.RelocationDebugWriter != nil && loc.Offset != InvalidOffset {
						var weakness string
						if loc.Type&reloctype.R_WEAK > 0 {
							weakness = "WEAK|"
						}
						relocType := weakness + objabi.RelocType(loc.Type&^reloctype.R_WEAK).String()
						_, _ = fmt.Fprintf(linker.options.RelocationDebugWriter, "DEDUPLICATING   %10s %10s %18s Base: 0x%x Pos: 0x%08x, Addr: 0x%016x AddrFromBase: %12d %s   to    %s\n",
							objabi.SymKind(symbol.Kind), objabi.SymKind(sym.Kind), relocType, addrBase, uintptr(unsafe.Pointer(&relocByte[loc.Offset])),
							addr, int(addr)-addrBase, symbol.Name, sym.Name)
					}
					switch loc.Type {
					case reloctype.R_PCREL:
						// The replaced t from another module will probably yield a massive negative offset, but that's ok as
						// PC-relative addressing is allowed to be negative (even if not very cache friendly)
						offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
						if offset > 0x7FFFFFFF || offset < -0x80000000 {
							err = fmt.Errorf("symName: %s offset: %d overflows!\n", sym.Name, offset)
						}
						byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
					case reloctype.R_CALLARM, reloctype.R_CALLARM64, reloctype.R_CALL:
						panic("This should not be possible")
					case reloctype.R_ADDRARM64, reloctype.R_ARM64_PCREL_LDST8, reloctype.R_ARM64_PCREL_LDST16, reloctype.R_ARM64_PCREL_LDST32, reloctype.R_ARM64_PCREL_LDST64:
						err2 := linker.relocateADRP(relocByte[loc.Offset:], loc, segment, addr)
						if err2 != nil {
							err = err2
						}
					case reloctype.R_ADDR, reloctype.R_WEAKADDR:
						// TODO - sanity check this
						address := uintptr(int(addr) + loc.Add)
						putAddress(byteorder, relocByte[loc.Offset:], uint64(address))
					case reloctype.R_ADDROFF, reloctype.R_WEAKADDROFF:
						offset := int(addr) - addrBase + loc.Add
						if offset > 0x7FFFFFFF || offset < -0x80000000 {
							err = fmt.Errorf("symName: %s %s offset: %d overflows!\n", objabi.RelocType(loc.Type), sym.Name, offset)
						}
						byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
					case reloctype.R_METHODOFF:
						if loc.Sym.Kind == symkind.STEXT {
							addrBase = segment.codeBase
						}
						offset := int(addr) - addrBase + loc.Add
						if offset > 0x7FFFFFFF || offset < -0x80000000 {
							err = fmt.Errorf("symName: %s %s offset: %d overflows!\n", objabi.RelocType(loc.Type), sym.Name, offset)
						}
						byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
					case reloctype.R_USETYPE, reloctype.R_USEIFACE, reloctype.R_USEIFACEMETHOD, reloctype.R_ADDRCUOFF, reloctype.R_KEEP:
						// nothing to do
					default:
						panic(fmt.Sprintf("unhandled reloc %s", objabi.RelocType(loc.Type)))
						// TODO - should we attempt to rewrite other relocations which point at *_types too?
					}
				}
			}
		}
	}
	codeModule.patchedTypeMethodsIfn = patchedTypeMethodsIfn
	codeModule.patchedTypeMethodsTfn = patchedTypeMethodsTfn
	codeModule.deduplicatedTypes = dedupedTypes

	if err != nil {
		return err
	}
	err = patchTypeMethodTextPtrs(uintptr(codeModule.codeBase), codeModule.patchedTypeMethodsIfn, codeModule.patchedTypeMethodsTfn)

	return err
}

func (linker *Linker) buildExports(codeModule *CodeModule, symbolMap map[string]uintptr, globalSymPtr map[string]uintptr) {
	codeModule.SymbolsByPkg = map[string]map[string]interface{}{}
	for _, pkg := range linker.pkgs {
		pkgSyms := map[string]interface{}{}
		for name, info := range pkg.Exports {
			typeAddr, ok := symbolMap[info.TypeName]
			if !ok {
				panic("could not find type symbol " + info.TypeName)
			}
			addr, ok := symbolMap[info.SymName]
			if !ok {
				panic(fmt.Sprintf("could not find symbol %s in package %s", info.SymName, pkg.PkgPath))
			}
			t := (*_type)(unsafe.Pointer(typeAddr))
			if dup, ok := codeModule.deduplicatedTypes[info.TypeName]; ok {
				t = (*_type)(unsafe.Pointer(dup))
			}

			var val interface{}
			valp := (*[2]unsafe.Pointer)(unsafe.Pointer(&val))
			(*valp)[0] = unsafe.Pointer(t)

			if t.Kind() == reflect.Func {
				(*valp)[1] = unsafe.Pointer(&addr)
			} else {
				(*valp)[1] = unsafe.Pointer(addr)
			}

			pkgSyms[name] = val
		}
		if len(pkgSyms) > 0 {
			codeModule.SymbolsByPkg[pkg.PkgPath] = pkgSyms
		}
	}
}

func (linker *Linker) UnresolvedExternalSymbols(symbolMap map[string]uintptr, ignorePackages []string, stdLibPkgs map[string]struct{}, unsafeBlindlyUseFirstModuleTypes bool) map[string]*obj.Sym {
	symMap := make(map[string]*obj.Sym)
	for symName, sym := range linker.symMap {
		shouldSkipDedup := false
		for _, pkgPath := range ignorePackages {
			if sym.Pkg == pkgPath {
				shouldSkipDedup = true
			}
		}
		if sym.Offset == InvalidOffset || shouldSkipDedup {
			if strings.HasPrefix(symName, TypePrefix) &&
				!strings.HasPrefix(symName, TypeDoubleDotPrefix) {
				// Always force the rebuild of non-std lib types in case they've changed between firstmodule and JIT code
				// They can be checked for structural equality if the JIT code builds it, but not if we blindly use the firstmodule version of a _type
				if typeSym, ok := symbolMap[symName]; ok {
					t := (*_type)(unsafe.Pointer(typeSym))
					_, isStdLibPkg := stdLibPkgs[t.PkgPath()]
					// Don't rebuild types in the stdlib, as these shouldn't be different (assuming same toolchain version for host and JIT)
					if t.PkgPath() != "" && !isStdLibPkg {
						// Only rebuild types which are reachable (via relocs) from the main package, otherwise we'll end up building everything unnecessarily
						if _, ok := linker.reachableTypes[symName]; ok && !unsafeBlindlyUseFirstModuleTypes {
							symMap[symName] = sym
						}
					}
				}
			}
			if _, ok := symbolMap[symName]; !ok || shouldSkipDedup {
				if _, ok := linker.objsymbolMap[symName]; !ok || shouldSkipDedup {
					symMap[symName] = sym
				}
			}
		}
	}
	return symMap
}

func (linker *Linker) UnresolvedPackageReferences(existingPkgs []string) []string {
	var pkgList []string
	for pkgName := range linker.pkgNamesWithUnresolved {
		for _, existing := range existingPkgs {
			if pkgName == existing {
				continue
			}
		}
		pkgList = append(pkgList, pkgName)
	}
	return pkgList
}

func (linker *Linker) UnresolvedExternalSymbolUsers(symbolMap map[string]uintptr) map[string][]string {
	requiredBy := map[string][]string{}
	for symName, sym := range linker.symMap {
		if sym.Offset == InvalidOffset {
			if _, ok := symbolMap[symName]; !ok {
				if _, ok := linker.objsymbolMap[symName]; !ok {
					var requiredBySet = map[string]struct{}{}
					for _, otherSym := range linker.symMap {
						for _, reloc := range otherSym.Reloc {
							if reloc.Sym.Name == symName {
								requiredBySet[otherSym.Name] = struct{}{}
							}
						}
					}
					requiredByList := make([]string, 0, len(requiredBySet))
					for k := range requiredBySet {
						requiredByList = append(requiredByList, k)
					}
					sort.Strings(requiredByList)
					requiredBy[sym.Name] = requiredByList
				}
			}
		}
	}
	return requiredBy
}

func (linker *Linker) UnloadStrings() {
	linker.heapStringMap = nil
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
	codeModule.maxCodeLength = alignof(codeModule.codeLen, PageSize)
	codeModule.maxDataLength = alignof(codeModule.sumDataLen, PageSize)
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

	var symbolMap map[string]uintptr
	if symbolMap, err = linker.addSymbolMap(symPtr, codeModule); err == nil {
		if err = linker.relocate(codeModule, symbolMap); err == nil {
			if err = linker.buildModule(codeModule, symbolMap); err == nil {
				if err = linker.deduplicateTypeDescriptors(codeModule, symbolMap); err == nil {
					linker.buildExports(codeModule, symbolMap, symPtr)
					MakeThreadJITCodeExecutable(uintptr(codeModule.codeBase), codeModule.maxCodeLength)
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
		if err2 != nil {
			err = fmt.Errorf("failed to munmap (%s) after linker error: %w", err2, err)
		}
		if err3 != nil {
			err = fmt.Errorf("failed to munmap (%s) after linker error: %w", err3, err)
		}
	}
	return nil, err
}

func (cm *CodeModule) Unload() error {
	err := cm.revertPatchedTypeMethods()
	if err != nil {
		return err
	}
	removeitabs(cm.module)
	runtime.GC()
	modulesLock.Lock()
	removeModule(cm)
	modulesLock.Unlock()
	modulesinit()
	err1 := Munmap(cm.codeByte)
	err2 := Munmap(cm.dataByte)
	if err1 != nil {
		return err1
	}
	cm.heapStrings = nil
	return err2
}

func (cm *CodeModule) TextAddr() (start, end uintptr) {
	if cm.module == nil {
		return 0, 0
	}
	return cm.module.text, cm.module.etext
}

func (cm *CodeModule) DataAddr() (start, end uintptr) {
	if cm.module == nil {
		return 0, 0
	}
	return cm.module.data, cm.module.edata
}
