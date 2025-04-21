package goloader

import (
	"github.com/pkujhd/goloader/libdl"
)

func (linker *Linker) RegisterCgoSymbols(symPtr map[string]uintptr) error {
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
		symPtr[cgoImport.GoSymName] = ptr
	}
	return nil
}
