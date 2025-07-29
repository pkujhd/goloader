package link

import (
	"fmt"
	"github.com/pkujhd/goloader/constants"
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

func (linker *Linker) readObj(file, pkgPath string) error {
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), CgoImports: make(map[string]*obj.CgoImport, 0), File: file, PkgPath: pkgPath}
	if pkg.PkgPath == constants.EmptyString {
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

	pkg.AddCgoFuncs(linker.CgoFuncs)
	linker.Packages[pkg.PkgPath] = &pkg
	return nil
}

func (linker *Linker) resolveSymbols() {
	for _, pkg := range linker.Packages {
		pkg.AddSymIndex(linker.CgoFuncs)
	}
	for _, pkg := range linker.Packages {
		pkg.ResolveSymbols(linker.Packages, linker.ObjSymbolMap, linker.CUOffset)
		pkg.GoArchive = nil
		pkg.Syms = nil
		linker.addFiles(pkg.CUFiles)
		for name, cgoImport := range pkg.CgoImports {
			linker.CgoImportMap[name] = cgoImport
		}
	}
}

func ReadObj(file, pkgpath string) (*Linker, error) {
	linker := initLinker()
	if err := linker.readObj(file, pkgpath); err != nil {
		return nil, err
	}
	linker.resolveSymbols()
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
	linker.resolveSymbols()
	linker.initPcHeader()
	if err := linker.addSymbols(); err != nil {
		return nil, err
	}
	return linker, nil
}

func ReadDependPackages(linker *Linker, files, pkgPaths []string, symbolNames []string, symPtr map[string]uintptr) error {
	if linker.AdaptedOffset {
		return fmt.Errorf("already adapted symbol offset, don't add new symbols")
	}

	linker.ObjSymbolMap = make(map[string]*obj.ObjSymbol)

	for i, file := range files {
		if err := linker.readObj(file, pkgPaths[i]); err != nil {
			return err
		}
	}
	linker.resolveSymbols()

	for _, name := range symbolNames {
		if _, ok := linker.ObjSymbolMap[name]; ok {
			_, err := linker.addSymbol(name, symPtr)
			if err != nil {
				return err
			}
		}
	}

	for _, pkg := range linker.Packages {
		name := getInitFuncName(pkg.PkgPath)
		if _, ok := linker.ObjSymbolMap[name]; ok {
			if symbol, ok := linker.SymMap[name]; !ok || symbol.Offset == constants.InvalidOffset {
				if !isCompleteInitialization(linker, name, symPtr) {
					_, err := linker.addSymbol(name, symPtr)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	unimplementedTypes := checkUnimplementedInterface(linker, symPtr)
	if unimplementedTypes != nil {
		unresolvedSymbols := make([]string, len(unimplementedTypes))
		for name := range unimplementedTypes {
			unresolvedSymbols = append(unresolvedSymbols, name)
			delete(symPtr, name)
		}
		for _, name := range unresolvedSymbols {
			if _, ok := linker.ObjSymbolMap[name]; ok {
				_, err := linker.addSymbol(name, symPtr)
				if err != nil {
					return err
				}
			}
		}
	}

	for name, _ := range linker.CgoImportMap {
		if _, ok := linker.SymMap[name]; !ok {
			delete(linker.CgoImportMap, name)
		}
	}

	linker.ObjSymbolMap = nil
	return nil
}
