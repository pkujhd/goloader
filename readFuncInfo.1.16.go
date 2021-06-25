// +build go1.16
// +build !go1.17

package goloader

import (
	"cmd/objfile/goobj"
)

func readFuncInfo(funcinfo *goobj.FuncInfo, b []byte) {
	funcinfo.Read(b)
}
