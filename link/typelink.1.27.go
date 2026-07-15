//go:build go1.27 && !go1.28
// +build go1.27,!go1.28

package link

import (
	"unsafe"

	"github.com/pkujhd/goloader/constants"
)

//go:linkname _DescriptorSize internal/abi.(*Type).DescriptorSize
func _DescriptorSize(t *_type) int
func (t *_type) DescriptorSize() int { return _DescriptorSize(t) }

// !IMPORTANT: only init firstmodule type, avoid load multiple objs but unload non-sequence errors
func typelinksRegister(symPtr map[string]uintptr) {
	md := firstmoduledata
	p := md.types
	p += constants.PtrSize

	for p < md.types+md.typedesclen {
		p = alignUp(p, constants.PtrSize)
		typ := (*_type)(unsafe.Pointer(p))
		registerType(typ, symPtr)
		p = p + uintptr(typ.DescriptorSize())
	}
}

func (linker *Linker) AddTypeLink(codeModule *CodeModule) {
	codeModule.module.typedesclen = uintptr(len(linker.NoPtrTypeData))
}
