// +build go1.9
// +build !go1.15

package goloader

import (
	"cmd/objfile/goobj"
	"unsafe"
)

func readPCInline(codeReloc *CodeReloc, symbol *goobj.Sym, fd *readAtSeeker) {
	fd.ReadAtWithSize(&(codeReloc.pclntable), symbol.Func.PCInline.Size, symbol.Func.PCInline.Offset)
	for _, inl := range symbol.Func.InlTree {
		if _, ok := codeReloc.namemap[inl.Func.Name]; !ok {
			codeReloc.namemap[inl.Func.Name] = len(codeReloc.pclntable)
			codeReloc.pclntable = append(codeReloc.pclntable, []byte(inl.Func.Name)...)
			codeReloc.pclntable = append(codeReloc.pclntable, ZERO_BYTE)
		}
	}
}

func findFuncNameOff(codereloc *CodeReloc, funcname string) int32 {
	return int32(codereloc.namemap[funcname])
}

func findFileTab(codereloc *CodeReloc, filename string) int32 {
	tab := codereloc.namemap[filename]
	for index, value := range codereloc.filetab {
		if uint32(tab) == value {
			return int32(index)
		}
	}
	return -1
}

func _addInlineTree(codereloc *CodeReloc, _func *_func, funcdata *[]uintptr, pcdata *[]uint32, inlineOffset uint32) (err error) {
	funcname := gostringnocopy(&codereloc.pclntable[_func.nameoff])
	Func := codereloc.symMap[funcname].Func
	if Func != nil && len(Func.InlTree) != 0 {
		name := funcname + INLINETREE_SUFFIX
		bytes := make([]byte, len(Func.InlTree)*InlinedCallSize)
		for k, inl := range Func.InlTree {
			inlinedcall := initInlinedCall(codereloc, inl, _func)
			copy2Slice(bytes[k*InlinedCallSize:], uintptr(unsafe.Pointer(&inlinedcall)), InlinedCallSize)
		}
		codereloc.stkmaps[name] = bytes
		for _func.nfuncdata <= _FUNCDATA_InlTree {
			*funcdata = append(*funcdata, uintptr(0))
			_func.nfuncdata++
		}
		(*funcdata)[_FUNCDATA_InlTree] = (uintptr)(unsafe.Pointer(&(codereloc.stkmaps[name][0])))
		for _func.npcdata <= _PCDATA_InlTreeIndex {
			*pcdata = append(*pcdata, uint32(0))
			_func.npcdata++
		}
		(*pcdata)[_PCDATA_InlTreeIndex] = inlineOffset
	}
	return err
}
