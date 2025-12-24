//go:build go1.20 && !go1.27
// +build go1.20,!go1.27

package obj

import (
	"cmd/objfile/goobj"
)

func readFuncInfo(funcinfo *goobj.FuncInfo, b []byte, info *FuncInfo) {
	lengths := funcinfo.ReadFuncInfoLengths(b)

	funcinfo.Args = funcinfo.ReadArgs(b)
	funcinfo.Locals = funcinfo.ReadLocals(b)
	funcinfo.FuncID = funcinfo.ReadFuncID(b)
	funcinfo.FuncFlag = funcinfo.ReadFuncFlag(b)
	funcinfo.StartLine = funcinfo.ReadStartLine(b)

	funcinfo.File = make([]goobj.CUFileIndex, lengths.NumFile)
	for i := range funcinfo.File {
		funcinfo.File[i] = funcinfo.ReadFile(b, lengths.FileOff, uint32(i))
	}
	funcinfo.InlTree = make([]goobj.InlTreeNode, lengths.NumInlTree)
	for i := range funcinfo.InlTree {
		funcinfo.InlTree[i] = funcinfo.ReadInlTree(b, lengths.InlTreeOff, uint32(i))
	}
	info.Args = funcinfo.Args
	info.Locals = funcinfo.Locals
	info.FuncID = uint8(funcinfo.FuncID)
	info.FuncFlag = uint8(funcinfo.FuncFlag)
	info.StartLine = funcinfo.StartLine
}
