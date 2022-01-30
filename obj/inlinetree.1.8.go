//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package obj

import (
	"cmd/objfile/goobj"
)

func initInline(objFunc *goobj.Func, Func *FuncInfo, pkgpath string, fd *readAtSeeker) (err error) {
	return nil
}
