//go:build go1.16 && !go1.19
// +build go1.16,!go1.19

package goloader

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
)

func initLinker() *Linker {
	linker := &Linker{
		symMap:       make(map[string]*obj.Sym),
		objsymbolMap: make(map[string]*obj.ObjSymbol),
		namemap:      make(map[string]int),
	}
	head := make([]byte, unsafe.Sizeof(pcHeader{}))
	copy(head, obj.ModuleHeadx86)
	linker.pclntable = append(linker.pclntable, head...)
	linker.pclntable[len(obj.ModuleHeadx86)-1] = PtrSize
	return linker
}
