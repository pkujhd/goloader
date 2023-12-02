//go:build go1.9 && !go1.12
// +build go1.9,!go1.12

package obj

// inlinedCall is the encoding of entries in the FUNCDATA_InlTree table.
type InlinedCall struct {
	parent int32 // index of parent in the inltree, or < 0
	file   int32 // fileno index into filetab
	line   int32 // line number of the call site
	func_  int32 // offset into pclntab for name of called function
}

func InitInlinedCall(inl InlTreeNode, funcid uint8, namemap map[string]int, filetab []uint32) InlinedCall {
	return InlinedCall{
		parent: int32(inl.Parent),
		file:   FindFileTab(inl.File, namemap, filetab),
		line:   int32(inl.Line),
		func_:  int32(namemap[inl.Func])}
}
