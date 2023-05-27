package goloader

import (
	"cmd/objfile/objabi"
	"unsafe"

	"github.com/eh-steve/goloader/obj"
	"github.com/eh-steve/goloader/objabi/dataindex"
)

func (linker *Linker) addInlineTree(_f *_func, symbol *obj.ObjSymbol) (err error) {
	funcname := symbol.Name
	Func := symbol.Func
	sym := linker.symMap[funcname]
	if Func != nil && len(Func.InlTree) != 0 {
		for _f.npcdata <= dataindex.PCDATA_InlTreeIndex {
			sym.Func.PCData = append(sym.Func.PCData, uint32(0))
			_f.npcdata++
		}
		sym.Func.PCData[dataindex.PCDATA_InlTreeIndex] = uint32(len(linker.pctab))

		for _, reloc := range symbol.Reloc {
			if reloc.EpilogueOffset > 0 {
				linker.patchPCValuesForReloc(&symbol.Func.PCInline, reloc.Offset, reloc.EpilogueOffset, reloc.EpilogueSize)
			}
		}
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
			// No special functions can be inlined in current versions of Go - we'll panic if they are
			// If we can't find the inlined funcID, we assume it's FuncID_normal.
			var funcID = uint8(objabi.FuncID_normal)
			if inlSym, ok := linker.objsymbolMap[inl.Func]; ok {
				funcID = inlSym.Func.FuncID
				if funcID != 0 {
					// This should never happen
					panic("Unexpectedly non-zero funcID in inlined func: " + inl.Func + " inlined inside " + sym.Name)
				}
			}
			inlinedcall := obj.InitInlinedCall(inl, funcID, linker.namemap, linker.cutab)
			copy2Slice(bytes[k*obj.InlinedCallSize:], uintptr(unsafe.Pointer(&inlinedcall)), obj.InlinedCallSize)
		}
		offset := len(linker.noptrdata)
		linker.noptrdata = append(linker.noptrdata, bytes...)
		bytearrayAlign(&linker.noptrdata, PtrSize)
		for _f.nfuncdata <= dataindex.FUNCDATA_InlTree {
			sym.Func.FuncData = append(sym.Func.FuncData, uintptr(0))
			_f.nfuncdata++
		}
		sym.Func.FuncData[dataindex.FUNCDATA_InlTree] = (uintptr)(offset)
	}
	return err
}
