//go:build go1.9 && !go1.12
// +build go1.9,!go1.12

package goloader

import (
	"cmd/objfile/goobj"
	"strings"
)

// copy from $GOROOT/src/cmd/internal/objabi/symkind.go
const (
	// An otherwise invalid zero value for the type
	Sxxx = iota
	// Executable instructions
	STEXT
	// Read only static data
	SRODATA
	// Static data that does not contain any pointers
	SNOPTRDATA
	// Static data
	SDATA
	// Statically data that is initially all 0s
	SBSS
	// Statically data that is initially all 0s and does not contain pointers
	SNOPTRBSS
	// Thread-local data that is initally all 0s
	STLSBSS
	// Debugging data
	SDWARFINFO
	SDWARFRANGE
	SDWARFLOC
	SDWARFMISC
	// Update cmd/link/internal/sym/AbiSymKindToSymKind for new SymKind values.
)

func (linker *Linker) addStackObject(funcname string, symbolMap map[string]uintptr, module *moduledata) (err error) {
	return nil
}

func (linker *Linker) addDeferReturn(_func *_func) (err error) {
	return nil
}

// inlinedCall is the encoding of entries in the FUNCDATA_InlTree table.
type inlinedCall struct {
	parent int32 // index of parent in the inltree, or < 0
	file   int32 // fileno index into filetab
	line   int32 // line number of the call site
	func_  int32 // offset into pclntab for name of called function
}

func (linker *Linker) initInlinedCall(inl InlTreeNode, _func *_func) inlinedCall {
	return inlinedCall{
		parent: int32(inl.Parent),
		file:   findFileTab(linker, inl.File),
		line:   int32(inl.Line),
		func_:  int32(linker.namemap[inl.Func])}
}

func initInline(objFunc *goobj.Func, Func *FuncInfo, pkgpath string, fd *readAtSeeker) (err error) {
	for _, inl := range objFunc.InlTree {
		inline := InlTreeNode{
			Parent:   int64(inl.Parent),
			File:     inl.File,
			Line:     int64(inl.Line),
			Func:     inl.Func.Name,
			ParentPC: 0,
		}
		inline.Func = strings.Replace(inline.Func, EmptyPkgPath, pkgpath, -1)
		Func.InlTree = append(Func.InlTree, inline)
	}
	Func.PCInline, err = fd.BytesAt(objFunc.PCInline.Offset, objFunc.PCInline.Size)
	return err
}

func (linker *Linker) addInlineTree(_func *_func, objsym *ObjSymbol) (err error) {
	return linker._addInlineTree(_func, objsym)
}

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	codeModule.module.filetab = linker.filetab
}
