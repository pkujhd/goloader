//go:build go1.12 && !go1.20
// +build go1.12,!go1.20

package obj

type funcID uint8

// inlinedCall is the encoding of entries in the FUNCDATA_InlTree table.
type InlinedCall struct {
	parent   uint16 // index of parent in the inltree, or < 0
	funcID   funcID // type of the called function
	_        byte
	file     uint32 // fileno index into filetab
	line     uint32 // line number of the call site
	func_    uint32 // offset into pclntab for name of called function
	parentPc uint32 // position of an instruction whose source position is the call site (offset from entry)
}

func InitInlinedCall(inl InlTreeNode, funcid uint8, namemap map[string]int, filetab []uint32) InlinedCall {
	return InlinedCall{
		parent:   uint16(inl.Parent),
		funcID:   funcID(funcid),
		file:     findFileTab(inl.File, namemap, filetab),
		line:     uint32(inl.Line),
		func_:    uint32(namemap[inl.Func]),
		parentPc: uint32(inl.ParentPC)}
}
