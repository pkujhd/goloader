//go:build go1.9 && !go1.19
// +build go1.9,!go1.19

package goloader

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/dataindex"
)

func findFileTab(linker *Linker, filename string) int32 {
	tab := linker.namemap[filename]
	for index, value := range linker.filetab {
		if uint32(tab) == value {
			return int32(index)
		}
	}
	return -1
}

func (linker *Linker) addInlineTree(_func *_func, symbol *obj.ObjSymbol) (err error) {
	funcname := symbol.Name
	Func := symbol.Func
	sym := linker.symMap[funcname]
	if Func != nil && len(Func.InlTree) != 0 {
		for _func.npcdata <= dataindex.PCDATA_InlTreeIndex {
			sym.Func.PCData = append(sym.Func.PCData, uint32(0))
			_func.npcdata++
		}
		sym.Func.PCData[dataindex.PCDATA_InlTreeIndex] = uint32(len(linker.pclntable))

		linker.pclntable = append(linker.pclntable, symbol.Func.PCInline...)
		for _, inl := range symbol.Func.InlTree {
			if _, ok := linker.namemap[inl.Func]; !ok {
				linker.namemap[inl.Func] = len(linker.pclntable)
				linker.pclntable = append(linker.pclntable, []byte(inl.Func)...)
				linker.pclntable = append(linker.pclntable, ZeroByte)
			}
		}

		bytes := make([]byte, len(Func.InlTree)*InlinedCallSize)
		for k, inl := range Func.InlTree {
			inlinedcall := linker.initInlinedCall(inl, _func)
			copy2Slice(bytes[k*InlinedCallSize:], uintptr(unsafe.Pointer(&inlinedcall)), InlinedCallSize)
		}
		offset := len(linker.noptrdata)
		linker.noptrdata = append(linker.noptrdata, bytes...)
		bytearrayAlign(&linker.noptrdata, PtrSize)
		for _func.nfuncdata <= dataindex.FUNCDATA_InlTree {
			sym.Func.FuncData = append(sym.Func.FuncData, uintptr(0))
			_func.nfuncdata++
		}
		sym.Func.FuncData[dataindex.FUNCDATA_InlTree] = (uintptr)(offset)
	}
	return err
}
