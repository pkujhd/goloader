//go:build go1.8 && !go1.27
// +build go1.8,!go1.27

package link

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"unsafe"

	"github.com/pkujhd/goloader/constants"
)

func getTypeLinkDataInMacho(machoFile *macho.File) ([]byte, int, error) {
	typeLinkSect := machoFile.Section("__typelink")
	typeLinkSectData, err := typeLinkSect.Data()
	if err != nil {
		return nil, 0, err
	}

	return typeLinkSectData, len(typeLinkSectData) / constants.Uint32Size, nil
}

func getTypeLinkDataInElf(elfFile *elf.File) ([]byte, int, error) {
	typeLinkSect := elfFile.Section(".typelink")
	typeLinkSectData, err := typeLinkSect.Data()
	if err != nil {
		return nil, 0, err
	}
	return typeLinkSectData, len(typeLinkSectData) / constants.Uint32Size, nil
}

func getTypeLinkDataInPE(peFile *pe.File) ([]byte, int, error) {
	typelinkSym := getSymbolInPe(peFile, "runtime.typelink")
	moduledataSym := getSymbolInPe(peFile, "runtime.firstmoduledata")
	dataSect := peFile.Section(".data")
	dataSectData, err := dataSect.Data()
	if err != nil {
		return nil, 0, err
	}

	md := (*moduledata)(unsafe.Pointer(&dataSectData[moduledataSym.Value]))
	typelinksSectData, _ := peFile.Sections[typelinkSym.SectionNumber-1].Data()
	return typelinksSectData[typelinkSym.Value:], len(md.typelinks), nil

}

func registerTypelinksInExe(symPtr map[string]uintptr, data []byte, typelinks []int32, addr uintptr) {
	md := exeData.md
	md.types = uintptr(unsafe.Pointer(&data[0]))
	md.etypes = md.types + uintptr(len(data))
	md.text = uintptr(unsafe.Pointer(&(*exeData.textSectData)[0]))
	md.etext = md.text + uintptr(len(*exeData.textSectData))
	md.typelinks = typelinks

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
