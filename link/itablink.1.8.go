//go:build go1.8 && !go1.27
// +build go1.8,!go1.27

package link

import (
	"unsafe"
)

func additabs(module *moduledata) {
	lock(itabLock)
	for _, it := range module.itablinks {
		itabAdd(it)
	}
	unlock(itabLock)
}

func regsiterItablinks(symPtr map[string]uintptr) {
	module := firstmoduledata
	lock(itabLock)
	for _, it := range module.itablinks {
		symPtr[getItabName(it)] = uintptr(unsafe.Pointer(it))
	}
	unlock(itabLock)
}

func (linker *Linker) AddItabLink(codeModule *CodeModule, symbolMap map[string]uintptr) {
	for symbolName, _ := range linker.SymMap {
		//fill itablinks
		if isItabName(symbolName) {
			codeModule.module.itablinks = append(codeModule.module.itablinks, (*itab)(adduintptr(symbolMap[symbolName], 0)))
		}
	}
}
