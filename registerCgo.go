package goloader

import (
	"github.com/pkujhd/goloader/libdl"
)

func (linker *Linker) RegisterCgoSymbols(symPtr map[string]uintptr) error {
	soNameMap := make(map[string]string)
	for _, cgoImport := range linker.CgoImportMap {
		soNameMap[cgoImport.SoName] = cgoImport.SoName
	}

	for _, soName := range soNameMap {
		_, err := libdl.Open(soName)
		if err != nil {
			return err
		}
	}

	handle, err := libdl.Open(EmptyString)
	if err != nil {
		return err
	}

	for _, cgoImport := range linker.CgoImportMap {
		ptr, err := libdl.LookupSymbol(handle, cgoImport.CSymName)
		if err != nil {
			return err
		}
		symPtr[cgoImport.GoSymName] = ptr
	}
	return nil
}
