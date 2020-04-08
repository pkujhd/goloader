package goloader

import (
	"cmd/objfile/goobj"
	"fmt"
	"os"
	"strings"
)

func readObj(f *os.File, reloc *CodeReloc, objsymmap map[string]objSym, pkgpath *string) error {
	if pkgpath == nil || *pkgpath == "" {
		var defaultPkgPath = "main"
		pkgpath = &defaultPkgPath
	}
	obj, err := goobj.Parse(f, *pkgpath)
	if len(reloc.Arch) != 0 && reloc.Arch != obj.Arch {
		return fmt.Errorf("read obj error: Arch %s != Arch %s", reloc.Arch, obj.Arch)
	}
	reloc.Arch = obj.Arch
	if err != nil {
		return fmt.Errorf("read error: %v", err)
	}
	for _, sym := range obj.Syms {
		objsymmap[sym.Name] = objSym{
			sym:  sym,
			file: f,
		}
	}
	for _, sym := range obj.Syms {
		if sym.Kind == STEXT && sym.DupOK == false {
			relocSym(reloc, sym.Name, objsymmap)
		} else if sym.Kind == SRODATA {
			if strings.HasPrefix(sym.Name, "type.") {
				relocSym(reloc, sym.Name, objsymmap)
			}
		}
	}
	return nil
}

func ReadObj(f *os.File) (*CodeReloc, error) {
	reloc := CodeReloc{SymMap: make(map[string]int), GCObjs: make(map[string]uintptr), FileMap: make(map[string]int)}
	reloc.Mod.pclntable = append(reloc.Mod.pclntable, x86moduleHead...)
	var objsymmap = make(map[string]objSym)
	err := readObj(f, &reloc, objsymmap, nil)
	if err != nil {
		return nil, err
	}
	if reloc.Arch == "arm" || reloc.Arch == "arm64" {
		copy(reloc.Mod.pclntable, armmoduleHead)
	}
	return &reloc, err
}

func ReadObjs(files []string, pkgPath []string) (*CodeReloc, error) {
	reloc := CodeReloc{SymMap: make(map[string]int), GCObjs: make(map[string]uintptr), FileMap: make(map[string]int)}
	reloc.Mod.pclntable = append(reloc.Mod.pclntable, x86moduleHead...)
	var objsymmap = make(map[string]objSym)
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
	if reloc.Arch == "arm" || reloc.Arch == "arm64" {
		copy(reloc.Mod.pclntable, armmoduleHead)
	}
	return &reloc, nil
}
