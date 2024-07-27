package goloader

import (
	"fmt"

	"github.com/pkujhd/goloader/obj"
)

func Parse(file, pkgpath string) ([]string, error) {
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), File: file, PkgPath: pkgpath}
	if err := pkg.Symbols(); err != nil {
		return nil, err
	}
	symbols := make([]string, 0)
	for _, sym := range pkg.Syms {
		symbols = append(symbols, sym.Name)
	}
	return symbols, nil
}

func (linker *Linker) readObj(file, pkgpath string) error {
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), File: file, PkgPath: pkgpath}
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
			for index, inl := range sym.Func.InlTree {
				sym.Func.InlTree[index].Func = obj.ReplacePkgPath(inl.Func, pkg.PkgPath)
			}
			sym.Func.CUOffset += linker.CUOffset
		}
		sym.Name = obj.ReplacePkgPath(sym.Name, pkg.PkgPath)
		linker.ObjSymbolMap[sym.Name] = sym
	}
	linker.addFiles(pkg.CUFiles)
	linker.Packages[pkg.PkgPath] = &pkg
	return nil
}

func ReadObj(file, pkgpath string) (*Linker, error) {
	linker := initLinker()
	if err := linker.readObj(file, pkgpath); err != nil {
		return nil, err
	}
	linker.initPcHeader()
	if err := linker.addSymbols(); err != nil {
		return nil, err
	}
	return linker, nil
}

func ReadObjs(files []string, pkgPaths []string) (*Linker, error) {
	linker := initLinker()
	for i, file := range files {
		if err := linker.readObj(file, pkgPaths[i]); err != nil {
			return nil, err
		}
	}
	linker.initPcHeader()
	if err := linker.addSymbols(); err != nil {
		return nil, err
	}
	return linker, nil
}

func (linker *Linker) ReadDependPkg(file, pkgPath string, symbolNames []string, symPtr map[string]uintptr) error {
	if linker.AdaptedOffset {
		return fmt.Errorf("already adapted symbol offset, don't add new symbols")
	}
	//only add unresolved symbol in ObjSymbolMap. use temporary map store read symbols
	objSymbolMap := linker.ObjSymbolMap
	linker.ObjSymbolMap = make(map[string]*obj.ObjSymbol)
	if err := linker.readObj(file, pkgPath); err != nil {
		return err
	}
	initFuncName := getInitFuncName(pkgPath)
	if _, ok := linker.ObjSymbolMap[initFuncName]; ok {
		if _, err := linker.addSymbol(initFuncName, nil); err != nil {
			return err
		}
	}
	for _, name := range symbolNames {
		if _, ok := linker.ObjSymbolMap[name]; ok {
			delete(linker.SymMap, name)
			_, err := linker.addSymbol(name, symPtr)
			if err != nil {
				return err
			}
		}
	}
	for name, sym := range linker.ObjSymbolMap {
		if _, ok := linker.SymMap[name]; ok {
			objSymbolMap[name] = sym
		}
	}
	linker.ObjSymbolMap = objSymbolMap
	return nil
}
