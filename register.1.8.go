// +build go1.8
// +build !go1.16

package goloader

import (
	"strings"
	"unsafe"
)

func registerFunc(md *moduledata, symPtr map[string]uintptr) {
	//register function
	for _, f := range md.ftab {
		if int(f.funcoff) < len(md.pclntable) {
			_func := (*_func)(unsafe.Pointer((&(md.pclntable[f.funcoff]))))
			name := gostringnocopy(&(md.pclntable[_func.nameoff]))
			if !strings.HasPrefix(name, TypeDoubleDotPrefix) && _func.entry < md.etext {
				if _, ok := symPtr[name]; !ok {
					symPtr[name] = _func.entry
				}
			}
		}
	}
}
