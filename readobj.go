package goloader

import (
	"cmd/objfile/sys"
	"fmt"
	"os"
	"strings"
)

func readObj(f *os.File, reloc *CodeReloc, objSymMap map[string]*ObjSymbol, pkgpath *string) error {
	if pkgpath == nil || *pkgpath == EmptyString {
		defaultPkgPath := DefaultPkgPath
		pkgpath = &defaultPkgPath
	}
	objs, Arch, err := symbols(f, *pkgpath)
	if err != nil {
		return fmt.Errorf("read error: %v", err)
	}
	if len(reloc.Arch) != 0 && reloc.Arch != Arch {
		return fmt.Errorf("read obj error: Arch %s != Arch %s", reloc.Arch, Arch)
	}
	reloc.Arch = Arch
	for _, sym := range objs {
		objSymMap[sym.Name] = sym
		for index, loc := range sym.Reloc {
			sym.Reloc[index].Sym.Name = strings.Replace(loc.Sym.Name, EmptyPkgPath, *pkgpath, -1)
		}
		if sym.Func != nil {
			for index, FuncData := range sym.Func.FuncData {
				sym.Func.FuncData[index] = strings.Replace(FuncData, EmptyPkgPath, *pkgpath, -1)
			}
		}
	}
	return nil
}

func ReadObj(f *os.File) (*CodeReloc, error) {
	reloc := &CodeReloc{symMap: make(map[string]*Sym), stkmaps: make(map[string][]byte), namemap: make(map[string]int)}
	addPclntableHeader(reloc)
	objSymMap := make(map[string]*ObjSymbol)
	err := readObj(f, reloc, objSymMap, nil)
	if err != nil {
		return nil, err
	}
	//static_tmp is 0, golang compile not allocate memory.
	reloc.data = append(reloc.data, make([]byte, IntSize)...)
	for _, objSym := range objSymMap {
		if objSym.Kind == STEXT && objSym.DupOK == false {
			_, err := relocSym(reloc, objSym.Name, objSymMap)
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
	objSymMap := make(map[string]*ObjSymbol)
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
		if objSym.Kind == STEXT && objSym.DupOK == false {
			_, err := relocSym(reloc, objSym.Name, objSymMap)
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
