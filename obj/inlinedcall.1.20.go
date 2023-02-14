//go:build go1.20 && !go1.21
// +build go1.20,!go1.21

package obj

type funcID uint8

// / inlinedCall is the encoding of entries in the FUNCDATA_InlTree table.
type InlinedCall struct {
	funcID    funcID // type of the called function
	_         [3]byte
	nameOff   int32 // offset into pclntab for name of called function
	parentPc  int32 // position of an instruction whose source position is the call site (offset from entry)
	startLine int32 // line number of start of function (func keyword/TEXT directive)
}

func InitInlinedCall(inl InlTreeNode, funcid uint8, namemap map[string]int, filetab []uint32) InlinedCall {
	return InlinedCall{
		funcID:    funcID(funcid),
		startLine: int32(inl.Line),
		nameOff:   int32(namemap[inl.Func]),
		parentPc:  int32(inl.ParentPC)}
}
