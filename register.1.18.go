//go:build go1.18 && !go1.19
// +build go1.18,!go1.19

package goloader

import (
	"strings"
	"unsafe"
)

func registerFunc(md *moduledata, symPtr map[string]uintptr) {
	//register function
	for _, f := range md.ftab {
		if int(f.funcoff) < len(md.pclntable) {
			_func := (*_func)(unsafe.Pointer(&(md.pclntable[f.funcoff])))
			if int(_func.nameoff) > 0 && int(_func.nameoff) < len(md.funcnametab) {
				name := gostringnocopy(&(md.funcnametab[_func.nameoff]))
				if !strings.HasPrefix(name, TypeDoubleDotPrefix) && name != EmptyString {
					if _, ok := symPtr[name]; !ok {
						symPtr[name] = uintptr(adduintptr(md.text, int(_func.entryoff)))
					}
				}
			}
		}
	}
}
