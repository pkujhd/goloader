//go:build go1.16 && !go1.17
// +build go1.16,!go1.17

package goloader

import (
	"cmd/objfile/goobj"
)

func readFuncInfo(funcinfo *goobj.FuncInfo, b []byte, info *FuncInfo) {
	funcinfo.Read(b)
	info.Args = funcinfo.Args
	info.Locals = funcinfo.Locals
	info.FuncID = uint8(funcinfo.FuncID)
}
