package obj

import (
	"strings"

	"github.com/pkujhd/goloader/constants"
)

func FindFileTab(filename string, namemap map[string]int, filetab []uint32) int32 {
	tab := namemap[filename]
	for index, value := range filetab {
		if uint32(tab) == value {
			return int32(index)
		}
	}
	return -1
}

//go:inline
func Grow(bytes *[]byte, size int) {
	if len(*bytes) < size {
		*bytes = append(*bytes, make([]byte, size-len(*bytes))...)
	}
}

//go:inline
func ReplacePkgPath(name, pkgpath string) string {
	if !strings.HasPrefix(name, constants.TypeStringPrefix) {
		name = strings.Replace(name, constants.EmptyPkgPath, pkgpath, -1)
		//golang 1.13 - 1.19 go build -gcflags="-p xxx" xxx.go ineffective
		name = strings.Replace(name, constants.CommandLinePkgPath, pkgpath, -1)
	}
	return name
}

func replacePkgPath(sym *ObjSymbol, pkgpath string) {
	for index, loc := range sym.Reloc {
		sym.Reloc[index].SymName = ReplacePkgPath(loc.SymName, pkgpath)
	}
	if sym.Type != EmptyString {
		sym.Type = ReplacePkgPath(sym.Type, pkgpath)
	}
	if sym.Func != nil {
		for index, FuncData := range sym.Func.FuncData {
			sym.Func.FuncData[index] = ReplacePkgPath(FuncData, pkgpath)
		}
		for index, inl := range sym.Func.InlTree {
			sym.Func.InlTree[index].Func = ReplacePkgPath(inl.Func, pkgpath)
		}
	}
	sym.Name = ReplacePkgPath(sym.Name, pkgpath)
}

//go:inline
func isTypeName(aName string) bool {
	return strings.HasPrefix(aName, constants.TypePrefix) && !strings.HasPrefix(aName, constants.TypeDoubleDotPrefix)
}
