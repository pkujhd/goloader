package goloader

import (
	"cmd/objfile/objfile"
	"debug/elf"
	"debug/gosym"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

type typedata struct {
	data      []byte
	saddr     uintptr
	naddr     uintptr
	byteOrder binary.ByteOrder
	adapted   map[int32]int32
}

func (td *typedata) adaptPtr(dataOff int) uintptr {
	ptr := uintptr(td.byteOrder.Uint64(td.data[dataOff:]))
	if PtrSize == Uint32Size {
		ptr = uintptr(td.byteOrder.Uint32(td.data[dataOff:]))
	}
	putAddress(td.byteOrder, td.data[dataOff:], uint64(ptr+td.naddr-td.saddr))
	return ptr + td.naddr - td.saddr
}

func (td *typedata) adaptType(tl int32) {
	if _, ok := td.adapted[tl]; ok {
		return
	}
	td.adapted[tl] = tl
	t := (*_type)(adduintptr(td.naddr, int(tl)))
	switch t.Kind() {
	case reflect.Array, reflect.Ptr, reflect.Chan, reflect.Slice:
		//Element
		addr := td.adaptPtr(int(tl) + _typeSize)
		td.adaptType(int32(addr - td.naddr))
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
			td.adaptType(int32(addr - td.naddr))
		}
	case reflect.Interface:
		//pkgPath
		td.adaptPtr(int(tl + int32(_typeSize)))
		//imethod slice.Data
		td.adaptPtr(int(tl + int32(_typeSize+PtrSize)))
	case reflect.Map:
		//Key
		addr := td.adaptPtr(int(tl) + _typeSize)
		td.adaptType(int32(addr - td.naddr))
		//Elem
		addr = td.adaptPtr(int(tl) + _typeSize + PtrSize)
		td.adaptType(int32(addr - td.naddr))
		//Bucket
		addr = td.adaptPtr(int(tl) + _typeSize + PtrSize + PtrSize)
		td.adaptType(int32(addr - td.naddr))
	case reflect.Struct:
		//PkgPath
		td.adaptPtr(int(tl + int32(_typeSize)))
		s := (*sliceHeader)(unsafe.Pointer(&td.data[tl+int32(_typeSize+PtrSize)]))
		for i := 0; i < s.Len; i++ {
			//Filed Name
			off := s.Data - td.saddr + +uintptr(3*i)*PtrSize
			td.adaptPtr(int(off))
			//Field Type
			addr := td.adaptPtr(int(off + PtrSize))
			td.adaptType(int32(addr - td.naddr))
		}
		s.Data = s.Data + td.naddr - td.saddr
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
	machoFile, _ := macho.Open(path)
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

	section := machoFile.Sections[typesSym.Sect-1]
	sectData, err := section.Data()
	if err != nil {
		return err
	}

	byteOrder := machoFile.ByteOrder
	typelinks := *ptr2uint32slice(uintptr(unsafe.Pointer(&typeLinkSectData[0])), len(typeLinkSectData)/Uint32Size)
	_registerTypesInExe(symPtr, byteOrder, typelinks, sectData[typesSym.Value-section.Addr:], uintptr(typesSym.Value))
	return nil
}

func registerTypesInElf(path string, symPtr map[string]uintptr) error {
	elfFile, _ := elf.Open(path)
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

	section := elfFile.Sections[typesSym.Section-1]
	sectData, err := section.Data()
	if err != nil {
		return err
	}

	byteOrder := elfFile.ByteOrder
	typelinks := *ptr2uint32slice(uintptr(unsafe.Pointer(&typeLinkSectData[0])), len(typeLinkSectData)/Uint32Size)
	_registerTypesInExe(symPtr, byteOrder, typelinks, sectData[typesSym.Value-section.Addr:], uintptr(typesSym.Value))
	return nil
}

func registerTypesInPE(path string, symPtr map[string]uintptr) error {
	peFile, _ := pe.Open(path)
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

	roDataSect := peFile.Section(".rdata")
	roDataSectData, err := roDataSect.Data()
	if err != nil {
		return err
	}

	dataSect := peFile.Section(".data")
	dataSectData, err := dataSect.Data()
	if err != nil {
		return err
	}

	md := (*moduledata)(unsafe.Pointer(&dataSectData[moduledataSym.Value]))
	typelinks := *ptr2uint32slice(uintptr(unsafe.Pointer(&roDataSectData[typelinkSym.Value])), len(md.typelinks))

	getImageBase := func(peFile *pe.File) uintptr {
		_, pe64 := peFile.OptionalHeader.(*pe.OptionalHeader64)
		if pe64 {
			return uintptr(peFile.OptionalHeader.(*pe.OptionalHeader64).ImageBase)
		} else {
			return uintptr(peFile.OptionalHeader.(*pe.OptionalHeader32).ImageBase)
		}
	}
	_registerTypesInExe(symPtr, binary.LittleEndian, typelinks, roDataSectData, uintptr(roDataSect.VirtualAddress)+getImageBase(peFile))
	return nil
}

func _registerTypesInExe(symPtr map[string]uintptr, byteOrder binary.ByteOrder, typelinks []int32, data []byte, addr uintptr) {
	md := &moduledata{typelinks: typelinks}
	md.types = uintptr(unsafe.Pointer(&data[0]))
	md.etypes = md.types + uintptr(len(data))

	td := typedata{
		data:      data,
		saddr:     addr,
		naddr:     md.types,
		byteOrder: byteOrder,
		adapted:   make(map[int32]int32),
	}
	modulesLock.Lock()
	addModule(md)
	modulesLock.Unlock()
	for _, tl := range md.typelinks {
		td.adaptType(tl)
	}
	for _, tl := range md.typelinks {
		registerType((*_type)(adduintptr(md.types, int(tl))), symPtr)
	}
	modulesLock.Lock()
	removeModule(md)
	modulesLock.Unlock()
}

func registerTypesInExe(symPtr map[string]uintptr, path string) error {
	f, err := objfile.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	pcLineTable, err := f.PCLineTable()
	if err != nil {
		return err
	}
	for _, f := range pcLineTable.(*gosym.Table).Funcs {
		symPtr[f.Name] = uintptr(f.Entry)
	}

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
