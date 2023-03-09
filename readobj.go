package goloader

import (
	"cmd/objfile/sys"
	"fmt"
	"os"
	"strings"

	"github.com/pkujhd/goloader/obj"
)

func Parse(f *os.File, pkgpath *string) ([]string, error) {
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), F: f, PkgPath: *pkgpath}
	symbols := make([]string, 0)
	if err := pkg.Symbols(); err != nil {
		return symbols, err
	}
	for _, sym := range pkg.Syms {
		symbols = append(symbols, sym.Name)
	}
	return symbols, nil
}

func readObj(pkg *obj.Pkg, linker *Linker, cuOffset int) error {
	if pkg.PkgPath == EmptyString {
		pkg.PkgPath = DefaultPkgPath
	}
	if err := pkg.Symbols(); err != nil {
		return fmt.Errorf("read error: %v", err)
	}
	if linker.Arch != nil && linker.Arch.Name != pkg.Arch {
		return fmt.Errorf("read obj error: Arch %s != Arch %s", linker.Arch.Name, pkg.Arch)
	} else {
		linker.Arch = getArch(pkg.Arch)
	}
	switch linker.Arch.Name {
	case sys.ArchARM.Name, sys.ArchARM64.Name:
		copy(linker.pclntable, obj.ModuleHeadarm)
		linker.pclntable[len(obj.ModuleHeadarm)-1] = PtrSize
	}

	for _, sym := range pkg.Syms {
		for index, loc := range sym.Reloc {
			if !strings.HasPrefix(sym.Reloc[index].Sym.Name, TypeStringPrefix) {
				sym.Reloc[index].Sym.Name = strings.Replace(loc.Sym.Name, EmptyPkgPath, pkg.PkgPath, -1)
			}
		}
		if sym.Type != EmptyString {
			sym.Type = strings.Replace(sym.Type, EmptyPkgPath, pkg.PkgPath, -1)
		}
		if sym.Func != nil {
			for index, FuncData := range sym.Func.FuncData {
				sym.Func.FuncData[index] = strings.Replace(FuncData, EmptyPkgPath, pkg.PkgPath, -1)
			}
			sym.Func.CUOffset += int32(cuOffset)
		}
	}
	for _, sym := range pkg.Syms {
		linker.objsymbolMap[sym.Name] = sym
	}
	linker.initFuncs = append(linker.initFuncs, getInitFuncName(pkg.PkgPath))
	return nil
}

func ReadObj(f *os.File, pkgpath *string) (*Linker, error) {
	linker := initLinker()
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), F: f, PkgPath: *pkgpath}
	if err := readObj(&pkg, linker, 0); err != nil {
		return nil, err
	}
	linker.addFiles(pkg.CUFiles)
	if err := linker.addSymbols(); err != nil {
		return nil, err
	}
	return linker, nil
}

func ReadObjs(files []string, pkgPath []string) (*Linker, error) {
	linker := initLinker()
	cuOffset := 0
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), F: f, PkgPath: pkgPath[i]}
		if err := readObj(&pkg, linker, cuOffset); err != nil {
			return nil, err
		}
		linker.addFiles(pkg.CUFiles)
		cuOffset += len(pkg.CUFiles)
	}
	if err := linker.addSymbols(); err != nil {
		return nil, err
	}
	return linker, nil
}
