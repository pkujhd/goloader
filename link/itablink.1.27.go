//go:build go1.27 && !go1.28
// +build go1.27,!go1.28

package link

import (
	"unsafe"

	"github.com/pkujhd/goloader/constants"
)

// Size returns the size of the itab in memory.
func (it *itab) Size() int {
	size := int(unsafe.Sizeof(itab{}))
	if it.fun[0] == 0 {
		return size
	}
	return size + (len(it.inter.mhdr)-1)*constants.PtrSize
}

func additabs(module *moduledata) {
	lock(itabLock)
	p := module.types + module.itaboffset
	end := p + module.itabsize
	for p < end {
		it := (*itab)(unsafe.Pointer(p))
		itabAdd(it)
		p += uintptr(it.Size())
	}
	unlock(itabLock)
}

func regsiterItablinks(symPtr map[string]uintptr) {
	module := firstmoduledata
	lock(itabLock)
	p := module.types + module.itaboffset
	end := p + module.itabsize
	for p < end {
		it := (*itab)(unsafe.Pointer(p))
		symPtr[getItabName(it)] = p
		p += uintptr(it.Size())
	}
	unlock(itabLock)
}

func (linker *Linker) AddItabLink(codeModule *CodeModule, symbolMap map[string]uintptr) {
	module := codeModule.module
	module.itaboffset = uintptr(codeModule.noPtrTypeDataLen)
	module.itabsize = uintptr(codeModule.noPtrItabDataLen)
}
