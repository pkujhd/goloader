package goloader

import (
	"unsafe"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/dataindex"
)

func (linker *Linker) addInlineTree(_func *_func, symbol *obj.ObjSymbol) (err error) {
	funcname := symbol.Name
	Func := symbol.Func
	sym := linker.SymMap[funcname]
	if Func != nil && len(Func.InlTree) != 0 {
		for _func.Npcdata <= dataindex.PCDATA_InlTreeIndex {
			sym.Func.PCData = append(sym.Func.PCData, uint32(0))
			_func.Npcdata++
		}
		sym.Func.PCData[dataindex.PCDATA_InlTreeIndex] = uint32(len(linker.Pclntable))

		for _, reloc := range symbol.Reloc {
			if reloc.Epilogue.Size > 0 {
				patchPCValues(linker, &symbol.Func.PCInline, reloc)
			}
		}

		linker.Pclntable = append(linker.Pclntable, symbol.Func.PCInline...)
		for _, inl := range symbol.Func.InlTree {
			if _, ok := linker.NameMap[inl.Func]; !ok {
				linker.NameMap[inl.Func] = len(linker.Pclntable)
				linker.Pclntable = append(linker.Pclntable, []byte(inl.Func)...)
				linker.Pclntable = append(linker.Pclntable, ZeroByte)
			}
		}

		bytes := make([]byte, len(Func.InlTree)*obj.InlinedCallSize)
		for k, inl := range Func.InlTree {
			funcID := uint8(0)
			if _, ok := linker.ObjSymbolMap[inl.Func]; ok {
				funcID = linker.ObjSymbolMap[inl.Func].Func.FuncID
			}
			inlinedcall := obj.InitInlinedCall(inl, funcID, linker.NameMap, linker.Filetab)
			copy2Slice(bytes[k*obj.InlinedCallSize:], uintptr(unsafe.Pointer(&inlinedcall)), obj.InlinedCallSize)
		}
		offset := len(linker.Noptrdata)
		linker.Noptrdata = append(linker.Noptrdata, bytes...)
		bytearrayAlign(&linker.Noptrdata, PtrSize)
		for _func.Nfuncdata <= dataindex.FUNCDATA_InlTree {
			sym.Func.FuncData = append(sym.Func.FuncData, uintptr(0))
			_func.Nfuncdata++
		}
		sym.Func.FuncData[dataindex.FUNCDATA_InlTree] = (uintptr)(offset)
	}
	return err
}
