package link

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"reflect"
	"runtime"
	"unsafe"

	"github.com/pkujhd/goloader/constants"
)

type typeData struct {
	data        []byte
	addrBase    uintptr
	newAddrBase uintptr
	ptrMask     func(uintptr) uintptr
	byteOrder   binary.ByteOrder
	adapted     map[int32]int32
}

type exeFileData struct {
	md            *moduledata
	typesSectData *[]byte
	textSectData  *[]byte
	typeData
}

var exeData = exeFileData{md: nil, typesSectData: nil, textSectData: nil}

func (_typData *typeData) adaptPtr(dataOff int) uintptr {
	ptr := uintptr(_typData.byteOrder.Uint64(_typData.data[dataOff:]))
	if constants.PtrSize == constants.Uint32Size {
		ptr = uintptr(_typData.byteOrder.Uint32(_typData.data[dataOff:]))
	}
	if _typData.ptrMask != nil {
		ptr = _typData.ptrMask(ptr)
	}
	newPtr := ptr + _typData.newAddrBase - _typData.addrBase
	putAddress(_typData.byteOrder, _typData.data[dataOff:], uint64(newPtr))
	return newPtr
}

func (_typData *typeData) adaptType(tl int32) {
	if _, ok := _typData.adapted[tl]; ok {
		return
	}
	_typData.adapted[tl] = tl
	t := (*_type)(adduintptr(_typData.newAddrBase, int(tl)))
	switch t.Kind() {
	case reflect.Array, reflect.Ptr, reflect.Chan, reflect.Slice:
		//Element
		addr := _typData.adaptPtr(int(tl) + _typeSize)
		_typData.adaptType(int32(addr - _typData.newAddrBase))
	case reflect.Func:
		f := (*funcType)(unsafe.Pointer(t))
		inOutCount := f.inCount + f.outCount&(1<<15-1)
		uadd := funcTypeSize
		if f.tflag&tflagUncommon != 0 {
			uadd += uncommonTypeSize
		}
		uadd += int(tl)
		for i := 0; i < int(inOutCount); i++ {
			addr := _typData.adaptPtr(int(uadd + i*constants.PtrSize))
			_typData.adaptType(int32(addr - _typData.newAddrBase))
		}
	case reflect.Interface:
		//pkgPath
		_typData.adaptPtr(int(tl + int32(_typeSize)))
	case reflect.Map:
		//Key
		addr := _typData.adaptPtr(int(tl) + _typeSize)
		_typData.adaptType(int32(addr - _typData.newAddrBase))
		//Elem
		addr = _typData.adaptPtr(int(tl) + _typeSize + constants.PtrSize)
		_typData.adaptType(int32(addr - _typData.newAddrBase))
		//Bucket
		addr = _typData.adaptPtr(int(tl) + _typeSize + constants.PtrSize + constants.PtrSize)
		_typData.adaptType(int32(addr - _typData.newAddrBase))
	case reflect.Struct:
		//PkgPath
		_typData.adaptPtr(int(tl + int32(_typeSize)))
		s := (*sliceHeader)(unsafe.Pointer(&_typData.data[tl+int32(_typeSize+constants.PtrSize)]))
		if _typData.ptrMask != nil {
			s.Data = _typData.ptrMask(s.Data)
		}
		for i := 0; i < s.Len; i++ {
			//Filed Name
			off := s.Data - _typData.addrBase + uintptr(3*i)*constants.PtrSize
			_typData.adaptPtr(int(off))
			//Field Type
			addr := _typData.adaptPtr(int(off + constants.PtrSize))
			_typData.adaptType(int32(addr - _typData.newAddrBase))
		}
		s.Data = s.Data + _typData.newAddrBase - _typData.addrBase
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
	/*
		on osx/arm64, pointer in const-data segment has invalid high memory address,
		use pointer mask function rewrite it.
	*/
	if runtime.GOARCH == "arm64" {
		exeData.ptrMask = func(ptr uintptr) uintptr {
			return ptr&0xFFFFFFFF | uintptr(typesSection.Addr-uint64(typesSection.Offset))
		}
	}

	exeData.byteOrder = machoFile.ByteOrder
	exeData.typesSectData = &typesSectData
	exeData.textSectData = &textSectData
	exeData.md.typelinks = *ptr2int32slice(uintptr(unsafe.Pointer(&typeLinkSectData[0])), len(typeLinkSectData)/constants.Uint32Size)
	registerTypelinksInExe(symPtr, typesSectData[typesSym.Value-typesSection.Addr:], uintptr(typesSym.Value))
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

	exeData.byteOrder = elfFile.ByteOrder
	exeData.typesSectData = &typesSectData
	exeData.textSectData = &textSectData
	exeData.md.typelinks = *ptr2int32slice(uintptr(unsafe.Pointer(&typeLinkSectData[0])), len(typeLinkSectData)/constants.Uint32Size)
	registerTypelinksInExe(symPtr, typesSectData[typesSym.Value-typesSection.Addr:], uintptr(typesSym.Value))
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

	exeData.byteOrder = binary.LittleEndian
	exeData.md.typelinks = *ptr2int32slice(uintptr(unsafe.Pointer(&typelinksSectData[typelinkSym.Value])), len(md.typelinks))

	roDataAddr := uintptr(typesSect.VirtualAddress) + getImageBase(peFile)
	registerTypelinksInExe(symPtr, typesSectData[md.types-roDataAddr:], md.types)
	return nil
}

func registerTypelinksInExe(symPtr map[string]uintptr, data []byte, addr uintptr) {
	md := exeData.md
	md.types = uintptr(unsafe.Pointer(&data[0]))
	md.etypes = md.types + uintptr(len(data))
	md.text = uintptr(unsafe.Pointer(&(*exeData.textSectData)[0]))
	md.etext = md.text + uintptr(len(*exeData.textSectData))

	exeData.data = data
	exeData.addrBase = addr
	exeData.newAddrBase = md.types

	modulesLock.Lock()
	addModule(md)
	modulesLock.Unlock()
	for _, tl := range md.typelinks {
		exeData.adaptType(tl)
	}
	for _, tl := range md.typelinks {
		registerType((*_type)(adduintptr(md.types, int(tl))), symPtr)
	}
}

func registerTypesInExe(symPtr map[string]uintptr, path string) error {
	exeData.md = &moduledata{}
	exeData.ptrMask = nil
	exeData.adapted = make(map[int32]int32)

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
