//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package obj

//golang 1.8 not support inline
type InlinedCall struct {
}

func InitInlinedCall(inl InlTreeNode, funcid uint8, namemap map[string]int, filetab []uint32) InlinedCall {
	return InlinedCall{}
}
