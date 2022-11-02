package goloader

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/dataindex"
)

func (linker *Linker) addInlineTree(_func *_func, symbol *obj.ObjSymbol) (err error) {
	funcname := symbol.Name
	Func := symbol.Func
	sym := linker.symMap[funcname]
	if Func != nil && len(Func.InlTree) != 0 {
		for _func.npcdata <= dataindex.PCDATA_InlTreeIndex {
			sym.Func.PCData = append(sym.Func.PCData, uint32(0))
			_func.npcdata++
		}
		sym.Func.PCData[dataindex.PCDATA_InlTreeIndex] = uint32(len(linker.pctab))

		linker.pctab = append(linker.pctab, symbol.Func.PCInline...)
		for _, inl := range symbol.Func.InlTree {
			if _, ok := linker.namemap[inl.Func]; !ok {
				linker.namemap[inl.Func] = len(linker.funcnametab)
				linker.funcnametab = append(linker.funcnametab, []byte(inl.Func)...)
				linker.funcnametab = append(linker.funcnametab, ZeroByte)
			}
		}

		bytes := make([]byte, len(Func.InlTree)*obj.InlinedCallSize)
		for k, inl := range Func.InlTree {
			inlinedcall := obj.InitInlinedCall(inl, getfuncID(_func), linker.namemap, linker.cutab)
			copy2Slice(bytes[k*obj.InlinedCallSize:], uintptr(unsafe.Pointer(&inlinedcall)), obj.InlinedCallSize)
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
