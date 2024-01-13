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

func grow(bytes *[]byte, size int) {
	if len(*bytes) < size {
		*bytes = append(*bytes, make([]byte, size-len(*bytes))...)
	}
}

func ReplacePkgPath(name, pkgpath string) string {
	if !strings.HasPrefix(name, constants.TypeStringPrefix) {
		name = strings.Replace(name, constants.EmptyPkgPath, pkgpath, -1)
	}
	return name
}
