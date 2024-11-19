//go:build go1.8 && !go1.10
// +build go1.8,!go1.10

package goloader

import (
	"unsafe"
)

// layout of Itab known to compilers
// allocated in non-garbage-collected memory
// Needs to be in sync with
// ../cmd/compile/internal/gc/reflect.go:/^func.dumptypestructs.
type itab struct {
	inter  *interfacetype
	_type  *_type
	link   *itab
	bad    int32
	inhash int32      // has this itab been added to hash?
	fun    [1]uintptr // variable sized
}

// See: src/runtime/iface.go
const hashSize = 1009

//go:linkname hash runtime.hash
var hash [hashSize]*itab

//go:linkname ifaceLock runtime.ifaceLock
var ifaceLock mutex

//go:linkname additab runtime.additab
func additab(m *itab, locked, canfail bool)

func additabs(module *moduledata) {
	lock(&ifaceLock)
	for _, itab := range module.itablinks {
		if itab.inhash == 0 {
			additab(itab, true, false)
		}
	}
	unlock(&ifaceLock)
}

func removeitabs(module *moduledata) bool {
	lock(&ifaceLock)
	defer unlock(&ifaceLock)

	//the itab alloc by runtime.persistentalloc, can't free
	for index, h := range hash {
		last := h
		for m := h; m != nil; m = m.link {
			uintptrm := uintptr(unsafe.Pointer(m))
			inter := uintptr(unsafe.Pointer(m.inter))
			_type := uintptr(unsafe.Pointer(m._type))
			if (inter >= module.types && inter <= module.etypes) || (_type >= module.types && _type <= module.etypes) ||
				(uintptrm >= module.types && uintptrm <= module.etypes) {
				if m == h {
					hash[index] = m.link
				} else {
					last.link = m.link
				}
			}
			last = m
		}
	}
	return true
}
