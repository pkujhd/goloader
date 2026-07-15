//go:build go1.27 && !go1.28
// +build go1.27,!go1.28

package link

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"unsafe"

	"github.com/pkujhd/goloader/constants"
)

func getTypeLinkDataInMacho(machoFile *macho.File) ([]byte, int, error) {
	moduleSect := machoFile.Section("__go_module")
	moduleSectData, err := moduleSect.Data()
	if err != nil {
		return nil, 0, err
	}

	dataSect := machoFile.Section("__data")
	dataSectData, err := dataSect.Data()
	if err != nil {
		return nil, 0, err
	}

	md := (*moduledata)(unsafe.Pointer(&moduleSectData[0]))
	return dataSectData, int(md.typedesclen), nil
}

func getTypeLinkDataInElf(elfFile *elf.File) ([]byte, int, error) {
	moduleSect := elfFile.Section(".go.module")
	moduleSectData, err := moduleSect.Data()
	if err != nil {
		return nil, 0, err
	}

	dataSect := elfFile.Section(".data")
	dataSectData, err := dataSect.Data()
	if err != nil {
		return nil, 0, err
	}

	md := (*moduledata)(unsafe.Pointer(&moduleSectData[0]))
	return dataSectData, int(md.typedesclen), nil
}

func getTypeLinkDataInPE(peFile *pe.File) ([]byte, int, error) {
	mdSym := getSymbolInPe(peFile, "runtime.firstmoduledata")
	dataSect := peFile.Section(".data")
	dataSectData, err := dataSect.Data()
	if err != nil {
		return nil, 0, err
	}

	md := (*moduledata)(unsafe.Pointer(&dataSectData[mdSym.Value]))
	return dataSectData, int(md.typedesclen), nil
}

func registerTypelinksInExe(symPtr map[string]uintptr, data []byte, typelink []int32, addr uintptr) {
	md := exeData.md
	md.types = uintptr(unsafe.Pointer(&data[0]))
	md.etypes = md.types + uintptr(len(data))
	md.text = uintptr(unsafe.Pointer(&(*exeData.textSectData)[0]))
	md.etext = md.text + uintptr(len(*exeData.textSectData))
	md.typedesclen = uintptr(len(typelink))

	exeData.data = data
	exeData.addrBase = addr
	exeData.newAddrBase = md.types

	modulesLock.Lock()
	addModule(md)
	modulesLock.Unlock()

	p := uintptr(0) + constants.PtrSize
	for p < md.typedesclen {
		p = alignUp(p, constants.PtrSize)
		typ := (*_type)(unsafe.Pointer(&data[p]))
		exeData.adaptType(int32(p))
		p = p + uintptr(typ.DescriptorSize())
	}
	p = uintptr(0) + constants.PtrSize
	for p < md.typedesclen {
		p = alignUp(p, constants.PtrSize)
		typ := (*_type)(unsafe.Pointer(&data[p]))
		registerType((*_type)(adduintptr(md.types, int(p))), symPtr)
		p = p + uintptr(typ.DescriptorSize())
	}
}
