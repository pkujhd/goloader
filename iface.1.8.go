// +build go1.8
// +build !go1.10,!go1.11,!go1.12,!go1.13,!go1.14,!go1.15

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

//go:linkname itabhash runtime.itabhash
func itabhash(inter *interfacetype, typ *_type) uint32

func eraseiface(inter *interfacetype, typ *_type) bool {
	lock(&ifaceLock)
	defer unlock(&ifaceLock)
	h := itabhash(inter, typ)
	var m, last *itab = nil, nil
	for m = (*itab)(loadp(unsafe.Pointer(&hash[h]))); m != nil; m = m.link {
		if m.inter == inter && m._type == typ {
			if last == nil {
				atomicstorep(unsafe.Pointer(&hash[h]), unsafe.Pointer(nil))
			} else {
				last.link = m.link
			}
			return true
		}
		last = m
	}
	return false
}
