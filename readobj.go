package goloader

import (
	"cmd/objfile/goobj"
	"cmd/objfile/sys"
	"fmt"
	"os"
	"strings"
)

var (
	x86moduleHead = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x1, PtrSize}
	armmoduleHead = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x4, PtrSize}
)

func Parse(f *os.File, pkgpath *string) ([]string, error) {
	obj, err := goobj.Parse(f, *pkgpath)
	if err != nil {
		return nil, fmt.Errorf("read error: %v", err)
	}
	symbolNames := make([]string, 0)
	for _, sym := range obj.Syms {
		symbolNames = append(symbolNames, sym.Name)
	}
	return symbolNames, nil
}

func addPclntableHeader(reloc *CodeReloc) {
	reloc.pclntable = append(reloc.pclntable, x86moduleHead...)
}

func readObj(f *os.File, reloc *CodeReloc, objSymMap map[string]objSym, pkgpath *string) error {
	if pkgpath == nil || *pkgpath == EmptyString {
		defaultPkgPath := DefaultPkgPath
		pkgpath = &defaultPkgPath
	}
	obj, err := goobj.Parse(f, *pkgpath)
	if err != nil {
		return fmt.Errorf("read error: %v", err)
	}
	if len(reloc.Arch) != 0 && reloc.Arch != obj.Arch {
		return fmt.Errorf("read obj error: Arch %s != Arch %s", reloc.Arch, obj.Arch)
	}
	reloc.Arch = obj.Arch
	for _, sym := range obj.Syms {
		objSymMap[sym.Name] = objSym{
			sym:     sym,
			file:    f,
			pkgpath: *pkgpath,
		}
		for index, loc := range sym.Reloc {
			sym.Reloc[index].Sym.Name = strings.Replace(loc.Sym.Name, EmptyPkgPath, *pkgpath, -1)
		}
		if sym.Func != nil {
			for index, FuncData := range sym.Func.FuncData {
				sym.Func.FuncData[index].Sym.Name = strings.Replace(FuncData.Sym.Name, EmptyPkgPath, *pkgpath, -1)
			}
		}
	}
	return nil
}

func ReadObj(f *os.File) (*CodeReloc, error) {
	reloc := &CodeReloc{symMap: make(map[string]*Sym), stkmaps: make(map[string][]byte), namemap: make(map[string]int)}
	addPclntableHeader(reloc)
	objSymMap := make(map[string]objSym)
	err := readObj(f, reloc, objSymMap, nil)
	if err != nil {
		return nil, err
	}
	//static_tmp is 0, golang compile not allocate memory.
	reloc.data = append(reloc.data, make([]byte, IntSize)...)
	for _, objSym := range objSymMap {
		if objSym.sym.Kind == STEXT && objSym.sym.DupOK == false {
			_, err := relocSym(reloc, objSym.sym.Name, objSymMap)
			if err != nil {
				return nil, err
			}
		}
	}
	switch reloc.Arch {
	case sys.ArchARM.Name, sys.ArchARM64.Name:
		copy(reloc.pclntable, armmoduleHead)
	}
	return reloc, err
}

func ReadObjs(files []string, pkgPath []string) (*CodeReloc, error) {
	reloc := &CodeReloc{symMap: make(map[string]*Sym), stkmaps: make(map[string][]byte), namemap: make(map[string]int)}
	addPclntableHeader(reloc)
	objSymMap := make(map[string]objSym)
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		err = readObj(f, reloc, objSymMap, &(pkgPath[i]))
		if err != nil {
			return nil, err
		}
	}
	//static_tmp is 0, golang compile not allocate memory.
	reloc.data = append(reloc.data, make([]byte, IntSize)...)
	for _, objSym := range objSymMap {
		if objSym.sym.Kind == STEXT && objSym.sym.DupOK == false {
			_, err := relocSym(reloc, objSym.sym.Name, objSymMap)
			if err != nil {
				return nil, err
			}
		}
	}
	switch reloc.Arch {
	case sys.ArchARM.Name, sys.ArchARM64.Name:
		copy(reloc.pclntable, armmoduleHead)
	}
	return reloc, nil
}
