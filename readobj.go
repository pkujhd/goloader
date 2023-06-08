package goloader

import (
	"cmd/objfile/goobj"
	"cmd/objfile/sys"
	"fmt"
	"github.com/eh-steve/goloader/obj"
	"github.com/eh-steve/goloader/objabi/reloctype"
	"github.com/eh-steve/goloader/objabi/symkind"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"unsafe"
)

func Parse(f *os.File, pkgpath *string) ([]string, error) {
	pkg := obj.Pkg{Syms: make(map[string]*obj.ObjSymbol, 0),
		F:             f,
		PkgPath:       *pkgpath,
		SymNamesByIdx: make(map[uint32]string),
		Exports:       make(map[string]obj.ExportSymType),
	}
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
	SymbolNameOrder                  []string
	RandomSymbolNameOrder            bool
	RelocationDebugWriter            io.Writer
	DumpTextBeforeAndAfterRelocs     bool
	NoRelocationEpilogues            bool
	SkipTypeDeduplicationForPackages []string
	ForceTestRelocationEpilogues     bool
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

func WithDumpTextBeforeAndAfterRelocs() func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.DumpTextBeforeAndAfterRelocs = true
	}
}

func WithSkipTypeDeduplicationForPackages(packages []string) func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.SkipTypeDeduplicationForPackages = packages
	}
}

func WithForceTestRelocationEpilogues() func(*LinkerOptions) {
	return func(options *LinkerOptions) {
		options.ForceTestRelocationEpilogues = true
	}
}

func resolveSymRefName(symRef goobj.SymRef, pkgs []*obj.Pkg, objByPkg map[string]uint32, objIdx uint32) (symName, pkgName string) {
	pkg := pkgs[objIdx-1]
	pkgName = pkg.ReferencedPkgs[symRef.PkgIdx]
	fileIdx := objByPkg[pkgName]
	if fileIdx == 0 {
		return "", pkgName
	}
	return pkgs[fileIdx-1].SymNamesByIdx[symRef.SymIdx], pkgName
}

func ReadObjs(files []string, pkgPath []string, globalSymPtr map[string]uintptr, linkerOpts ...LinkerOptFunc) (*Linker, error) {
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
	objByPkg := map[string]uint32{}
	var pkgs = make([]*obj.Pkg, 0, len(files))
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		osFiles = append(osFiles, f)
		pkg := obj.Pkg{
			Syms:          make(map[string]*obj.ObjSymbol, 0),
			F:             f,
			PkgPath:       pkgPath[i],
			Objidx:        uint32(i + 1),
			SymNamesByIdx: make(map[uint32]string),
			Exports:       make(map[string]obj.ExportSymType),
		}
		objByPkg[pkgPath[i]] = pkg.Objidx
		if err := readObj(&pkg, linker); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, &pkg)
		symNames = append(symNames, pkg.SymNameOrder...)
	}

	for _, objSym := range linker.objsymbolMap {
		if strings.HasPrefix(objSym.Type, obj.UnresolvedSymRefPrefix) {
			// This type symbol was likely in another package and so was unresolved at the time of loading the archive,
			// but we might have added the missing package in a later archive, so try to resolve again.
			unresolved := objSym.Type
			var pkgName string
			symRef := obj.ParseUnresolvedIdxString(objSym.Type)
			objSym.Type, pkgName = resolveSymRefName(symRef, pkgs, objByPkg, objSym.Objidx)
			if objSym.Type == "" {
				// Still unresolved, add a fake invalid symbol entry and reloc for this symbol to prevent the linker progressing
				linker.symMap[pkgName+"."+unresolved] = &obj.Sym{Name: pkgName + "." + unresolved, Offset: InvalidOffset, Pkg: pkgName}
				objSym.Reloc = append(objSym.Reloc, obj.Reloc{Sym: linker.symMap[pkgName+"."+unresolved], Type: reloctype.R_KEEP})
				linker.pkgNamesWithUnresolved[pkgName] = struct{}{}
			}
		}
		if _, presentInFirstModule := globalSymPtr[objSym.Name]; presentInFirstModule {
			// If a symbol is reachable from the firstmodule, we should mark it as reachable for us too,
			// even if our package can't reach it, since the first module might call a previously unreachable method via our JIT module
			linker.collectReachableTypes(objSym.Name)
			linker.collectReachableSymbols(objSym.Name)
		}
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
	mainPkgSyms := pkgs[len(pkgs)-1].Syms

	for symName := range mainPkgSyms {
		linker.collectReachableTypes(symName)
	}
	for symName := range mainPkgSyms {
		linker.collectReachableSymbols(symName)
	}

	firstModuleTypesToForceRebuild := map[*_type]*obj.ObjSymbol{}

	firstmodule := activeModules()[0]
	for reachable := range linker.reachableSymbols {
		objSym := linker.objsymbolMap[reachable]
		if objSym == nil {
			continue
		}
		for _, reloc := range objSym.Reloc {
			// The new module uses an interface method - we need to check firstmodule's itabs to see whether any types
			// already implement this interface but whose method is unreachable, as there's a risk that the new module
			// could call one of these methods and hit a "fatal error: unreachable method called. linker bug?"
			if reloc.Type == reloctype.R_USEIFACEMETHOD {
				ifaceType := linker.objsymbolMap[reloc.Sym.Name]
				if ifaceType == nil {
					// Must be a firstmodule iface which we didn't rebuild
					for _, itab := range firstmodule.itablinks {
						if TypePrefix+resolveFullyQualifiedSymbolName(&itab.inter.typ) == reloc.Sym.Name {
							m := (*method)(add(unsafe.Pointer(itab.inter), uintptr(reloc.Add)))
							methodName := itab.inter.typ.nameOff(m.name).name()
							u := itab._type.uncommon()
							for _, method := range u.methods() {
								if itab._type.nameOff(method.name).name() == methodName && (method.ifn == -1 || method.tfn == -1) {
									firstModuleTypesToForceRebuild[itab._type] = objSym
								}
							}
						}
					}
				}
			}
		}
	}

outer:
	for t, objSym := range firstModuleTypesToForceRebuild {
		name := TypePrefix + resolveFullyQualifiedSymbolName(t)
		pkgName := t.PkgPath()
		for _, pkg := range linker.pkgs {
			if pkg.PkgPath == pkgName {
				continue outer
			}
		}
		if _, ok := linker.objsymbolMap[name]; !ok {
			linker.symMap[name] = &obj.Sym{Name: name, Offset: InvalidOffset, Pkg: pkgName, Kind: symkind.Sxxx}
			linker.objsymbolMap[name] = &obj.ObjSymbol{
				Name: name,
				Kind: symkind.Sxxx,
				Func: &obj.FuncInfo{},
				Pkg:  pkgName,
			}
			symNames = append(symNames, name)
			objSym.Reloc = append(objSym.Reloc, obj.Reloc{Sym: linker.symMap[name], Type: reloctype.R_KEEP})
		}
		linker.pkgNamesToForceRebuild[pkgName] = struct{}{}
		linker.collectReachableSymbols(name)
		linker.collectReachableTypes(name)
	}
	if err := linker.addSymbols(symNames, globalSymPtr); err != nil {
		return nil, err
	}
	linker.pkgs = pkgs
	return linker, nil
}

func (linker *Linker) isTypeReachable(symName string) bool {
	_, reachable := linker.reachableTypes[symName]
	return reachable
}

func (linker *Linker) isSymbolReachable(symName string) bool {
	_, reachable := linker.reachableSymbols[symName]
	return reachable
}

func (linker *Linker) collectReachableTypes(symName string) {
	if _, ok := linker.reachableTypes[symName]; ok {
		return
	}
	if strings.HasPrefix(symName, TypePrefix) && !strings.HasPrefix(symName, TypeDoubleDotPrefix) {
		linker.reachableTypes[symName] = struct{}{}
		if strings.HasPrefix(symName, TypePrefix+"*") {
			nonPtr := TypePrefix + strings.TrimPrefix(symName, TypePrefix+"*")
			linker.reachableTypes[nonPtr] = struct{}{}
			if nonPtrSym, ok := linker.symMap[nonPtr]; ok {
				for _, reloc := range nonPtrSym.Reloc {
					if strings.HasPrefix(reloc.Sym.Name, TypePrefix) && !strings.HasPrefix(reloc.Sym.Name, TypeDoubleDotPrefix) {
						linker.collectReachableTypes(reloc.Sym.Name)
					}
				}
			}
		}

		objsym := linker.objsymbolMap[symName]
		if objsym != nil {
			for _, reloc := range objsym.Reloc {
				if strings.HasPrefix(reloc.Sym.Name, TypePrefix) && !strings.HasPrefix(reloc.Sym.Name, TypeDoubleDotPrefix) {
					linker.collectReachableTypes(reloc.Sym.Name)
				}
			}
		}
	}
}

func (linker *Linker) collectReachableSymbols(symName string) {
	// Don't have to be as clever as linker's deadcode.go - just add everything we can reference conservatively
	if _, ok := linker.reachableSymbols[symName]; ok {
		return
	}

	linker.reachableSymbols[symName] = struct{}{}
	if strings.HasPrefix(symName, TypePrefix+"*") {
		nonPtr := TypePrefix + strings.TrimPrefix(symName, TypePrefix+"*")
		linker.collectReachableSymbols(nonPtr)
	}

	objsym := linker.objsymbolMap[symName]
	if objsym != nil {
		if objsym.Type != "" {
			linker.collectReachableSymbols(objsym.Type)
		}

		for _, reloc := range objsym.Reloc {
			linker.collectReachableSymbols(reloc.Sym.Name)
		}
		if objsym.Func != nil {
			for _, inl := range objsym.Func.InlTree {
				linker.collectReachableSymbols(inl.Func)
			}
			for _, funcData := range objsym.Func.FuncData {
				linker.collectReachableSymbols(funcData)
			}
		}
	}
}
