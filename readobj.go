package goloader

import (
	"fmt"
	"os"

	"github.com/pkujhd/goloader/obj"
)

func Parse(f *os.File, pkgpath *string) ([]string, error) {
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), File: f, PkgPath: *pkgpath}
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

	for _, sym := range pkg.Syms {
		for index, loc := range sym.Reloc {
			sym.Reloc[index].SymName = obj.ReplacePkgPath(loc.SymName, pkg.PkgPath)
		}
		if sym.Type != EmptyString {
			sym.Type = obj.ReplacePkgPath(sym.Type, pkg.PkgPath)
		}
		if sym.Func != nil {
			for index, FuncData := range sym.Func.FuncData {
				sym.Func.FuncData[index] = obj.ReplacePkgPath(FuncData, pkg.PkgPath)
			}
			sym.Func.CUOffset += int32(cuOffset)
		}
		sym.Name = obj.ReplacePkgPath(sym.Name, pkg.PkgPath)
		linker.ObjSymbolMap[sym.Name] = sym
	}
	linker.Packages = append(linker.Packages, pkg)
	return nil
}

func ReadObj(f *os.File, pkgpath *string) (*Linker, error) {
	linker := initLinker()
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), File: f, PkgPath: *pkgpath}
	if err := readObj(&pkg, linker, 0); err != nil {
		return nil, err
	}
	linker.addFiles(pkg.CUFiles)
	linker.initPcHeader()
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
		pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), File: f, PkgPath: pkgPath[i]}
		if err := readObj(&pkg, linker, cuOffset); err != nil {
			return nil, err
		}
		linker.addFiles(pkg.CUFiles)
		cuOffset += len(pkg.CUFiles)
	}
	linker.initPcHeader()
	if err := linker.addSymbols(); err != nil {
		return nil, err
	}
	return linker, nil
}
