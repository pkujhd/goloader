package goloader

import (
	"runtime"

	"github.com/pkujhd/goloader/libdl"
	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/symkind"
)

func (linker *Linker) AddCgoSymbols(symPtr map[string]uintptr) error {
	if len(linker.CgoImportMap) == 0 {
		return nil
	}

	soNameMap := make(map[string]string)
	for _, cgoImport := range linker.CgoImportMap {
		soNameMap[cgoImport.SoName] = cgoImport.SoName
	}

	soMap := make(map[string]uintptr, 0)
	for _, soName := range soNameMap {
		h, err := libdl.Open(soName)
		if err != nil {
			return err
		} else {
			soMap[soName] = h
		}
	}

	for _, cgoImport := range linker.CgoImportMap {
		ptr, err := libdl.LookupSymbol(soMap[cgoImport.SoName], cgoImport.CSymName)
		if err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			sym := obj.Sym{
				Name:   cgoImport.GoSymName,
				Kind:   symkind.SNOPTRDATA,
				Offset: len(linker.Noptrdata),
			}
			linker.Noptrdata = append(linker.Noptrdata, make([]byte, PtrSize)...)
			linker.SymMap[sym.Name] = &sym
			putAddress(linker.Arch.ByteOrder, linker.Noptrdata[sym.Offset:], uint64(ptr))
		} else {
			symPtr[cgoImport.GoSymName] = ptr
		}
	}

	return nil
}
