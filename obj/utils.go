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

//go:inline
func IsHasTypePrefix(name string) bool {
	return strings.HasPrefix(name, constants.TypePrefix)
}

//go:inline
func IsHasItabPrefix(name string) bool {
	return strings.HasPrefix(name, constants.ItabPrefix)
}

//go:inline
func IsHasStringPrefix(name string) bool {
	return strings.HasPrefix(name, constants.TypeStringPrefix)
}
