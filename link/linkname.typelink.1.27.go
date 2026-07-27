//go:build go1.27 && !go1.28
// +build go1.27,!go1.28

package link

import (
	"unsafe"
)

var (
	_DescriptorSize func(t *_type) int = nil

	moduleToTypelinksLock *mutex                    = nil
	moduleToTypelinks     *map[*moduledata][]*_type = nil
)

func addTypeLinkLinkName(symPtr map[string]uintptr) {
	*(*uintptr)(unsafe.Pointer(&_DescriptorSize)) = getFuncPointer(symPtr, "internal/abi.(*Type).DescriptorSize")

	moduleToTypelinksLock = (*mutex)(getVarPointer(symPtr, "runtime.moduleToTypelinksLock"))
	moduleToTypelinks = (*map[*moduledata][]*_type)(getVarPointer(symPtr, "runtime.moduleToTypelinks"))
}
