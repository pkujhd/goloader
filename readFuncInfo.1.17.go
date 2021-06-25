// +build go1.17
// +build !go1.18

package goloader

import (
	"cmd/objfile/goobj"
)

func readFuncInfo(funcinfo *goobj.FuncInfo, b []byte) {
	lengths := funcinfo.ReadFuncInfoLengths(b)

	funcinfo.Args = funcinfo.ReadArgs(b)
	funcinfo.Locals = funcinfo.ReadLocals(b)
	funcinfo.FuncID = funcinfo.ReadFuncID(b)

	funcinfo.Pcsp = funcinfo.ReadPcsp(b)
	funcinfo.Pcfile = funcinfo.ReadPcfile(b)
	funcinfo.Pcline = funcinfo.ReadPcline(b)
	funcinfo.Pcinline = funcinfo.ReadPcinline(b)
	funcinfo.Pcdata = funcinfo.ReadPcdata(b)

	funcinfo.Funcdataoff = make([]uint32, lengths.NumFuncdataoff)
	for i := range funcinfo.Funcdataoff {
		funcinfo.Funcdataoff[i] = uint32(funcinfo.ReadFuncdataoff(b, lengths.FuncdataoffOff, uint32(i)))
	}
	funcinfo.File = make([]goobj.CUFileIndex, lengths.NumFile)
	for i := range funcinfo.File {
		funcinfo.File[i] = funcinfo.ReadFile(b, lengths.FileOff, uint32(i))
	}
	funcinfo.InlTree = make([]goobj.InlTreeNode, lengths.NumInlTree)
	for i := range funcinfo.InlTree {
		funcinfo.InlTree[i] = funcinfo.ReadInlTree(b, lengths.InlTreeOff, uint32(i))
	}
}
