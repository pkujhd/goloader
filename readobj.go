package goloader

import (
	"cmd/objfile/goobj"
	"fmt"
	"os"
)

func readObj(f *os.File, reloc *CodeReloc, objsymmap map[string]objSym, pkgpath *string) error {
	if pkgpath == nil || *pkgpath == EMPTY_STRING {
		defaultPkgPath := DEFAULT_PKGPATH
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
		objsymmap[sym.Name] = objSym{
			sym:  sym,
			file: f,
		}
	}
	for _, sym := range obj.Syms {
		if sym.Kind == STEXT && sym.DupOK == false {
			_, err := relocSym(reloc, sym.Name, objsymmap)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func ReadObj(f *os.File) (*CodeReloc, error) {
	reloc := CodeReloc{SymMap: make(map[string]int), GCObjs: make(map[string]uintptr), FileMap: make(map[string]int)}
	reloc.Mod.pclntable = append(reloc.Mod.pclntable, x86moduleHead...)
	objsymmap := make(map[string]objSym)
	err := readObj(f, &reloc, objsymmap, nil)
	if err != nil {
		return nil, err
	}
	if reloc.Arch == ARCH_ARM32 || reloc.Arch == ARCH_ARM64 {
		copy(reloc.Mod.pclntable, armmoduleHead)
	}
	return &reloc, err
}

func ReadObjs(files []string, pkgPath []string) (*CodeReloc, error) {
	reloc := CodeReloc{SymMap: make(map[string]int), GCObjs: make(map[string]uintptr), FileMap: make(map[string]int)}
	reloc.Mod.pclntable = append(reloc.Mod.pclntable, x86moduleHead...)
	objsymmap := make(map[string]objSym)
	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		err = readObj(f, &reloc, objsymmap, &(pkgPath[i]))
		if err != nil {
			return nil, err
		}
	}
	if reloc.Arch == ARCH_ARM32 || reloc.Arch == ARCH_ARM64 {
		copy(reloc.Mod.pclntable, armmoduleHead)
	}
	return &reloc, nil
}
