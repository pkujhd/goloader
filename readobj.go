package goloader

import (
	"cmd/objfile/sys"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"

	"github.com/eh-steve/goloader/obj"
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

func readObj(pkg *obj.Pkg, linker *Linker) error {
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
		copy(linker.functab, obj.ModuleHeadarm)
		linker.functab[len(obj.ModuleHeadarm)-1] = PtrSize
	}

	cuOffset := 0
	for _, cuFiles := range linker.cuFiles {
		cuOffset += len(cuFiles.Files)
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
			sym.Func.CUOffset += cuOffset
		}
	}
	for _, sym := range pkg.Syms {
		linker.objsymbolMap[sym.Name] = sym
	}
	linker.cuFiles = append(linker.cuFiles, pkg.CUFiles...)
	linker.initFuncs = append(linker.initFuncs, getInitFuncName(pkg.PkgPath))
	return nil
}

type LinkerOptFunc func(options *LinkerOptions)

type LinkerOptions struct {
	HeapStrings                      bool
	StringContainerSize              int
	SymbolNameOrder                  []string
	RandomSymbolNameOrder            bool
	RelocationDebugWriter            io.Writer
	NoRelocationEpilogues            bool
	SkipTypeDeduplicationForPackages []string
}

func WithHeapStrings() func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.HeapStrings = true
	}
}

func WithStringContainer(size int) func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.StringContainerSize = size
	}
}

// WithSymbolNameOrder allows you to control the sequence (placement in memory) of symbols from an object file.
// When not set, the order as parsed from the archive file is used.
func WithSymbolNameOrder(symNames []string) func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.SymbolNameOrder = symNames
	}
}

func WithRandomSymbolNameOrder() func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.RandomSymbolNameOrder = true
	}
}

func WithRelocationDebugWriter(writer io.Writer) func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.RelocationDebugWriter = writer
	}
}

func WithNoRelocationEpilogues() func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.NoRelocationEpilogues = true
	}
}

func WithSkipTypeDeduplicationForPackages(packages []string) func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.SkipTypeDeduplicationForPackages = packages
	}
}

func ReadObj(f *os.File, pkgpath *string, linkerOpts ...LinkerOptFunc) (*Linker, error) {
	linker, err := initLinker(linkerOpts)
	if err != nil {
		return nil, err
	}
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), F: f, PkgPath: *pkgpath}
	if err := readObj(&pkg, linker); err != nil {
		return nil, err
	}

	symNames := pkg.SymNameOrder
	if len(linker.options.SymbolNameOrder) > 0 {
		if len(pkg.SymNameOrder) == len(linker.options.SymbolNameOrder) {
			isOk := true
			for _, symName := range linker.options.SymbolNameOrder {
				if _, ok := linker.objsymbolMap[symName]; !ok {
					isOk = false
				}
			}
			if isOk {
				log.Printf("linker using provided symbol name order for %d symbols", len(linker.options.SymbolNameOrder))
				symNames = linker.options.SymbolNameOrder
			}
		}
	}
	if linker.options.RandomSymbolNameOrder {
		rand.Shuffle(len(symNames), func(i, j int) {
			symNames[i], symNames[j] = symNames[j], symNames[i]
		})
	}
	if err := linker.addSymbols(symNames); err != nil {
		return nil, err
	}
	return linker, nil
}

func ReadObjs(files []string, pkgPath []string, linkerOpts ...LinkerOptFunc) (*Linker, error) {
	linker, err := initLinker(linkerOpts)
	if err != nil {
		return nil, err
	}
	var osFiles []*os.File
	defer func() {
		for _, f := range osFiles {
			_ = f.Close()
		}
	}()
	var symNames []string
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		osFiles = append(osFiles, f)
		pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0), F: f, PkgPath: pkgPath[i]}
		if err := readObj(&pkg, linker); err != nil {
			return nil, err
		}
		symNames = append(symNames, pkg.SymNameOrder...)
	}
	if len(linker.options.SymbolNameOrder) > 0 {
		if len(symNames) == len(linker.options.SymbolNameOrder) {
			isOk := true
			for _, symName := range linker.options.SymbolNameOrder {
				if _, ok := linker.objsymbolMap[symName]; !ok {
					isOk = false
				}
			}
			if isOk {
				log.Printf("linker using provided symbol name order for %d symbols", len(linker.options.SymbolNameOrder))
				symNames = linker.options.SymbolNameOrder
			}
		}
	}
	if linker.options.RandomSymbolNameOrder {
		rand.Shuffle(len(symNames), func(i, j int) {
			symNames[i], symNames[j] = symNames[j], symNames[i]
		})
	}
	if err := linker.addSymbols(symNames); err != nil {
		return nil, err
	}
	return linker, nil
}
