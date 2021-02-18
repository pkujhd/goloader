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

func Parse(f *os.File, pkgpath *string) ([]string, error) {
	pkg := Pkg{Syms: make(map[string]*ObjSymbol, 0), f: f, PkgPath: *pkgpath}
	symbols := make([]string, 0)
	if err := pkg.symbols(); err != nil {
		return symbols, err
	}
	for _, sym := range pkg.Syms {
		symbols = append(symbols, sym.Name)
	}
	return symbols, nil
}

func readObj(pkg *Pkg, linker *Linker) error {
	if pkg.PkgPath == EmptyString {
		pkg.PkgPath = DefaultPkgPath
	}
	if err := pkg.symbols(); err != nil {
		return fmt.Errorf("read error: %v", err)
	}
	if len(linker.Arch) != 0 && linker.Arch != pkg.Arch {
		return fmt.Errorf("read obj error: Arch %s != Arch %s", linker.Arch, pkg.Arch)
	} else {
		linker.Arch = pkg.Arch
	}
	switch linker.Arch {
	case sys.ArchARM.Name, sys.ArchARM64.Name:
		copy(linker.pclntable, armmoduleHead)
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
		linker.objsymbolMap[sym.Name] = sym
	}
	return nil
}

func ReadObj(f *os.File, pkgpath *string) (*Linker, error) {
	linker := initLinker()
	pkg := Pkg{Syms: make(map[string]*ObjSymbol, 0), f: f, PkgPath: *pkgpath}
	if err := readObj(&pkg, linker); err != nil {
		return nil, err
	}
	if err := linker.addSymbols(); err != nil {
		return nil, err
	}
	return linker, nil
}

func ReadObjs(files []string, pkgPath []string) (*Linker, error) {
	linker := initLinker()
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		pkg := Pkg{Syms: make(map[string]*ObjSymbol, 0), f: f, PkgPath: pkgPath[i]}
		if err := readObj(&pkg, linker); err != nil {
			return nil, err
		}
	}
	if err := linker.addSymbols(); err != nil {
		return nil, err
	}
	return linker, nil
}
