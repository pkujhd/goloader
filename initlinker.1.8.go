//go:build go1.8 && !go1.16
// +build go1.8,!go1.16

package goloader

import (
	"github.com/pkujhd/goloader/obj"
)

func initLinker() *Linker {
	linker := &Linker{
		symMap:       make(map[string]*obj.Sym),
		objsymbolMap: make(map[string]*obj.ObjSymbol),
		namemap:      make(map[string]int),
	}
	linker.pclntable = append(linker.pclntable, obj.ModuleHeadx86...)
	linker.pclntable[len(obj.ModuleHeadx86)-1] = PtrSize
	return linker
}
