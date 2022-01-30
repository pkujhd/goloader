//go:build go1.9 && !go1.12
// +build go1.9,!go1.12

package goloader

import (
	"github.com/pkujhd/goloader/obj"
)

// inlinedCall is the encoding of entries in the FUNCDATA_InlTree table.
type inlinedCall struct {
	parent int32 // index of parent in the inltree, or < 0
	file   int32 // fileno index into filetab
	line   int32 // line number of the call site
	func_  int32 // offset into pclntab for name of called function
}

func (linker *Linker) initInlinedCall(inl obj.InlTreeNode, _func *_func) inlinedCall {
	return inlinedCall{
		parent: int32(inl.Parent),
		file:   findFileTab(linker, inl.File),
		line:   int32(inl.Line),
		func_:  int32(linker.namemap[inl.Func])}
}
