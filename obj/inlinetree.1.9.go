//go:build go1.9 && !go1.12
// +build go1.9,!go1.12

package obj

import (
	"cmd/objfile/goobj"
)

func initInline(objFunc *goobj.Func, Func *FuncInfo, pkgpath string, fd *readAtSeeker) (err error) {
	for _, inl := range objFunc.InlTree {
		inline := InlTreeNode{
			Parent:   int64(inl.Parent),
			File:     inl.File,
			Line:     int64(inl.Line),
			Func:     inl.Func.Name,
			ParentPC: 0,
		}
		inline.Func = ReplacePkgPath(inline.Func, pkgpath)
		Func.InlTree = append(Func.InlTree, inline)
	}
	Func.PCInline, err = fd.BytesAt(objFunc.PCInline.Offset, objFunc.PCInline.Size)
	return err
}
