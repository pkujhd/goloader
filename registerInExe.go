package goloader

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

type typeData struct {
	data      []byte
	sAddr     uintptr
	nAddr     uintptr
	byteOrder binary.ByteOrder
	adapted   map[int32]int32
}

type exeFileData struct {
	md            *moduledata
	typesSectData *[]byte
	textSectData  *[]byte
	typData       *typeData
}

var exeData = exeFileData{md: nil, typesSectData: nil, textSectData: nil, typData: nil}

func (td *typeData) adaptPtr(dataOff int) uintptr {
	ptr := uintptr(td.byteOrder.Uint64(td.data[dataOff:]))
	if PtrSize == Uint32Size {
		ptr = uintptr(td.byteOrder.Uint32(td.data[dataOff:]))
	}
	putAddress(td.byteOrder, td.data[dataOff:], uint64(ptr+td.nAddr-td.sAddr))
	return ptr + td.nAddr - td.sAddr
}

func (td *typeData) adaptType(tl int32) {
	if _, ok := td.adapted[tl]; ok {
		return
	}
	td.adapted[tl] = tl
	t := (*_type)(adduintptr(td.nAddr, int(tl)))
	switch t.Kind() {
	case reflect.Array, reflect.Ptr, reflect.Chan, reflect.Slice:
		//Element
		addr := td.adaptPtr(int(tl) + _typeSize)
		td.adaptType(int32(addr - td.nAddr))
	case reflect.Func:
		f := (*funcType)(unsafe.Pointer(t))
		inOutCount := f.inCount + f.outCount&(1<<15-1)
		uadd := funcTypeSize
		if f.tflag&tflagUncommon != 0 {
			uadd += uncommonTypeSize
		}
		uadd += int(tl)
		for i := 0; i < int(inOutCount); i++ {
			addr := td.adaptPtr(int(uadd + i*PtrSize))
			td.adaptType(int32(addr - td.nAddr))
		}
	case reflect.Interface:
		//pkgPath
		td.adaptPtr(int(tl + int32(_typeSize)))
	case reflect.Map:
		//Key
		addr := td.adaptPtr(int(tl) + _typeSize)
		td.adaptType(int32(addr - td.nAddr))
		//Elem
		addr = td.adaptPtr(int(tl) + _typeSize + PtrSize)
		td.adaptType(int32(addr - td.nAddr))
		//Bucket
		addr = td.adaptPtr(int(tl) + _typeSize + PtrSize + PtrSize)
		td.adaptType(int32(addr - td.nAddr))
	case reflect.Struct:
		//PkgPath
		td.adaptPtr(int(tl + int32(_typeSize)))
		s := (*sliceHeader)(unsafe.Pointer(&td.data[tl+int32(_typeSize+PtrSize)]))
		for i := 0; i < s.Len; i++ {
			//Filed Name
			off := s.Data - td.sAddr + +uintptr(3*i)*PtrSize
			td.adaptPtr(int(off))
			//Field Type
			addr := td.adaptPtr(int(off + PtrSize))
			td.adaptType(int32(addr - td.nAddr))
		}
		s.Data = s.Data + td.nAddr - td.sAddr
	case reflect.Bool,
		reflect.Int, reflect.Uint,
		reflect.Int64, reflect.Uint64,
		reflect.Int32, reflect.Uint32,
		reflect.Int16, reflect.Uint16,
		reflect.Int8, reflect.Uint8,
		reflect.Float64, reflect.Float32,
		reflect.Complex64, reflect.Complex128,
		reflect.String, reflect.UnsafePointer,
		reflect.Uintptr:
		//noting todo
	default:
		panic(fmt.Errorf("not deal reflect type:%s", t.Kind()))
	}
}

func registerTypesInMacho(path string, symPtr map[string]uintptr) error {
	machoFile, err := macho.Open(path)
	if err != nil {
		return err
	}
	defer machoFile.Close()
	typeLinkSect := machoFile.Section("__typelink")
	typeLinkSectData, err := typeLinkSect.Data()
	if err != nil {
		return err
	}

	getSymbolInMacho := func(machoFile *macho.File, symbolName string) *macho.Symbol {
		symbols := machoFile.Symtab.Syms
		for _, sym := range symbols {
			if sym.Name == symbolName {
				return &sym
			}
		}
		return nil
	}
	typesSym := getSymbolInMacho(machoFile, "runtime.types")

	typesSection := machoFile.Sections[typesSym.Sect-1]
	typesSectData, err := typesSection.Data()
	if err != nil {
		return err
	}

	textSym := getSymbolInMacho(machoFile, "runtime.text")

	textSection := machoFile.Sections[textSym.Sect-1]
	textSectData, err := textSection.Data()
	if err != nil {
		return err
	}
	byteOrder := machoFile.ByteOrder

	exeData.typesSectData = &typesSectData
	exeData.textSectData = &textSectData
	exeData.md.typelinks = *ptr2uint32slice(uintptr(unsafe.Pointer(&typeLinkSectData[0])), len(typeLinkSectData)/Uint32Size)
	registerTypelinksInExe(symPtr, byteOrder, typesSectData[typesSym.Value-typesSection.Addr:], uintptr(typesSym.Value))
	return nil
}

func registerTypesInElf(path string, symPtr map[string]uintptr) error {
	elfFile, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer elfFile.Close()
	typeLinkSect := elfFile.Section(".typelink")
	typeLinkSectData, err := typeLinkSect.Data()
	if err != nil {
		return err
	}
	getSymbolInElf := func(elfFile *elf.File, symbolName string) *elf.Symbol {
		symbols, _ := elfFile.Symbols()
		for _, sym := range symbols {
			if sym.Name == symbolName {
				return &sym
			}
		}
		return nil
	}
	typesSym := getSymbolInElf(elfFile, "runtime.types")

	typesSection := elfFile.Sections[typesSym.Section]
	typesSectData, err := typesSection.Data()
	if err != nil {
		return err
	}
	textSym := getSymbolInElf(elfFile, "runtime.text")

	textSection := elfFile.Sections[textSym.Section]
	textSectData, err := textSection.Data()
	if err != nil {
		return err
	}

	byteOrder := elfFile.ByteOrder

	exeData.typesSectData = &typesSectData
	exeData.textSectData = &textSectData
	exeData.md.typelinks = *ptr2uint32slice(uintptr(unsafe.Pointer(&typeLinkSectData[0])), len(typeLinkSectData)/Uint32Size)
	registerTypelinksInExe(symPtr, byteOrder, typesSectData[typesSym.Value-typesSection.Addr:], uintptr(typesSym.Value))
	return nil
}

func registerTypesInPE(path string, symPtr map[string]uintptr) error {
	peFile, err := pe.Open(path)
	if err != nil {
		return err
	}
	defer peFile.Close()
	getSymbolInPe := func(symbols []*pe.Symbol, symbolName string) *pe.Symbol {
		for _, sym := range symbols {
			if sym.Name == symbolName {
				return sym
			}
		}
		return nil
	}
	typelinkSym := getSymbolInPe(peFile.Symbols, "runtime.typelink")
	moduledataSym := getSymbolInPe(peFile.Symbols, "runtime.firstmoduledata")
	typesSym := getSymbolInPe(peFile.Symbols, "runtime.types")
	textSym := getSymbolInPe(peFile.Symbols, "runtime.text")

	dataSect := peFile.Section(".data")
	dataSectData, err := dataSect.Data()
	if err != nil {
		return err
	}

	md := (*moduledata)(unsafe.Pointer(&dataSectData[moduledataSym.Value]))
	typelinksSectData, _ := peFile.Sections[typelinkSym.SectionNumber-1].Data()

	getImageBase := func(peFile *pe.File) uintptr {
		_, pe64 := peFile.OptionalHeader.(*pe.OptionalHeader64)
		if pe64 {
			return uintptr(peFile.OptionalHeader.(*pe.OptionalHeader64).ImageBase)
		} else {
			return uintptr(peFile.OptionalHeader.(*pe.OptionalHeader32).ImageBase)
		}
	}

	typesSect := peFile.Sections[typesSym.SectionNumber-1]
	typesSectData, _ := typesSect.Data()
	exeData.typesSectData = &typesSectData

	textSect := peFile.Sections[textSym.SectionNumber-1]
	textSectData, _ := textSect.Data()
	exeData.textSectData = &textSectData

	exeData.md.typelinks = *ptr2uint32slice(uintptr(unsafe.Pointer(&typelinksSectData[typelinkSym.Value])), len(md.typelinks))

	roDataAddr := uintptr(typesSect.VirtualAddress) + getImageBase(peFile)
	registerTypelinksInExe(symPtr, binary.LittleEndian, typesSectData[md.types-roDataAddr:], md.types)
	return nil
}

func registerTypelinksInExe(symPtr map[string]uintptr, byteOrder binary.ByteOrder, data []byte, addr uintptr) {
	md := exeData.md
	md.types = uintptr(unsafe.Pointer(&data[0]))
	md.etypes = md.types + uintptr(len(data))
	md.text = uintptr(unsafe.Pointer(&(*exeData.textSectData)[0]))
	md.etext = md.text + uintptr(len(*exeData.textSectData))
	exeData.typData = &typeData{
		data:      data,
		sAddr:     addr,
		nAddr:     md.types,
		byteOrder: byteOrder,
		adapted:   make(map[int32]int32),
	}
	modulesLock.Lock()
	addModule(md)
	modulesLock.Unlock()
	for _, tl := range md.typelinks {
		exeData.typData.adaptType(tl)
	}
	for _, tl := range md.typelinks {
		registerType((*_type)(adduintptr(md.types, int(tl))), symPtr)
	}
}

func registerTypesInExe(symPtr map[string]uintptr, path string) error {
	exeData.md = &moduledata{}
	switch runtime.GOOS {
	case "linux", "android":
		return registerTypesInElf(path, symPtr)
	case "darwin":
		return registerTypesInMacho(path, symPtr)
	case "windows":
		return registerTypesInPE(path, symPtr)
	default:
		panic(fmt.Errorf("unsupported platform:%s", runtime.GOOS))
	}
}

func RegSymbolWithPath(symPtr map[string]uintptr, path string) error {
	/*
		register types and functions in an executable file, the address of symbol not used for relocation,
		just for builder check reachable
	*/
	err := regSymbol(symPtr, path, true)
	if err != nil {
		return err
	}
	return registerTypesInExe(symPtr, path)
}
