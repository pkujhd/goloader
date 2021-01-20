package goloader

import (
	"cmd/objfile/sys"
	"fmt"
	"os"
	"strings"
)

type Pkg struct {
	Syms    map[string]*ObjSymbol
	Arch    string
	PkgPath string
	f       *os.File
}

func readObj(pkg *Pkg, reloc *CodeReloc) error {
	if pkg.PkgPath == EmptyString {
		pkg.PkgPath = DefaultPkgPath
	}
	if err := pkg.symbols(); err != nil {
		return fmt.Errorf("read error: %v", err)
	}
	if len(reloc.Arch) != 0 && reloc.Arch != pkg.Arch {
		return fmt.Errorf("read obj error: Arch %s != Arch %s", reloc.Arch, pkg.Arch)
	} else {
		reloc.Arch = pkg.Arch
	}
	switch reloc.Arch {
	case sys.ArchARM.Name, sys.ArchARM64.Name:
		copy(reloc.pclntable, armmoduleHead)
	}
	for _, sym := range pkg.Syms {
		for index, loc := range sym.Reloc {
			sym.Reloc[index].Sym.Name = strings.Replace(loc.Sym.Name, EmptyPkgPath, pkg.PkgPath, -1)
		}
		if sym.Func != nil {
			for index, FuncData := range sym.Func.FuncData {
				sym.Func.FuncData[index] = strings.Replace(FuncData, EmptyPkgPath, pkg.PkgPath, -1)
			}
		}
	}
	for _, sym := range pkg.Syms {
		reloc.objsymbolMap[sym.Name] = sym
	}
	return nil
}

func relocateSymbols(reloc *CodeReloc) error {
	//static_tmp is 0, golang compile not allocate memory.
	reloc.data = append(reloc.data, make([]byte, IntSize)...)
	for _, objSym := range reloc.objsymbolMap {
		if objSym.Kind == STEXT && objSym.DupOK == false {
			_, err := relocSym(reloc, objSym.Name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func ReadObj(f *os.File, pkgpath *string) (*CodeReloc, error) {
	reloc := initCodeReloc()
	pkg := Pkg{Syms: make(map[string]*ObjSymbol, 0), f: f, PkgPath: *pkgpath}
	if err := readObj(&pkg, reloc); err != nil {
		return nil, err
	}
	if err := relocateSymbols(reloc); err != nil {
		return nil, err
	}
	return reloc, nil
}

func ReadObjs(files []string, pkgPath []string) (*CodeReloc, error) {
	reloc := initCodeReloc()
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		pkg := Pkg{Syms: make(map[string]*ObjSymbol, 0), f: f, PkgPath: pkgPath[i]}
		if err := readObj(&pkg, reloc); err != nil {
			return nil, err
		}
	}
	if err := relocateSymbols(reloc); err != nil {
		return nil, err
	}
	return reloc, nil
}
