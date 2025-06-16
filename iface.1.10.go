//go:build go1.10 && !go1.26
// +build go1.10,!go1.26

package goloader

import (
	"unsafe"
)

// layout of Itab known to compilers
// allocated in non-garbage-collected memory
// Needs to be in sync with
// ../cmd/compile/internal/gc/reflect.go:/^func.dumptabs.
type itab struct {
	inter *interfacetype
	_type *_type
	hash  uint32 // copy of _type.hash. Used for type switches.
	_     [4]byte
	fun   [1]uintptr // variable sized. fun[0]==0 means _type does not implement inter.
}

const itabInitSize = 512

// Note: change the formula in the mallocgc call in itabAdd if you change these fields.
type itabTableType struct {
	size    uintptr             // length of entries array. Always a power of 2.
	count   uintptr             // current number of filled entries.
	entries [itabInitSize]*itab // really [size] large
}

//go:linkname _itabTable runtime.itabTable
var _itabTable uintptr // pointer to current table

var itabTable *itabTableType = (*itabTableType)(unsafe.Pointer(_itabTable))

//go:linkname _itabLock runtime.itabLock
var _itabLock uintptr

var itabLock *mutex = (*mutex)(unsafe.Pointer(&_itabLock))

//go:linkname itabAdd runtime.itabAdd
func itabAdd(m *itab)

func additabs(module *moduledata) {
	lock(itabLock)
	for _, itab := range module.itablinks {
		itabAdd(itab)
	}
	unlock(itabLock)
}

func removeitabs(module *moduledata) bool {
	lock(itabLock)
	defer unlock(itabLock)

	for i := uintptr(0); i < itabTable.size; i++ {
		p := (**itab)(add(unsafe.Pointer(&itabTable.entries), i*PtrSize))
		m := (*itab)(loadp(unsafe.Pointer(p)))
		if m != nil {
			uintptrm := uintptr(unsafe.Pointer(m))
			inter := uintptr(unsafe.Pointer(m.inter))
			_type := uintptr(unsafe.Pointer(m._type))
			if (inter >= module.types && inter <= module.etypes) || (_type >= module.types && _type <= module.etypes) ||
				(uintptrm >= module.types && uintptrm <= module.etypes) {
				atomicstorep(unsafe.Pointer(p), unsafe.Pointer(nil))
				itabTable.count = itabTable.count - 1
			}
		}
	}
	return true
}

func addItab(m *itab) {
	lock(itabLock)
	itabAdd(m)
	unlock(itabLock)
}
