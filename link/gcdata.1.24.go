//go:build go1.24 && !go1.26
// +build go1.24,!go1.26

package link

import (
	"cmd/objfile/gcprog"
	"reflect"
	"unsafe"
)

func gcDataAddType(linker *Linker, w *gcprog.Writer, off int64, typ *_type) {
	ptrData := int64(typ.ptrdata) / int64(linker.Arch.PtrSize)
	switch typ.Kind() {
	case reflect.Array:
		element := rtypeOf(typ.Elem())
		n := int64(typ.Len())
		gcDataAddType(linker, w, off, element)
		if n > 1 {
			// Issue repeat for subsequent n-1 instances.
			elemSize := int64(element.size)
			w.ZeroUntil((off + elemSize) / int64(linker.Arch.PtrSize))
			w.Repeat(elemSize/int64(linker.Arch.PtrSize), n-1)
		}
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			fieldType := rtypeOf(typ.Field(i).Type)
			if fieldType.ptrdata == 0 {
				continue
			}
			gcDataAddType(linker, w, off+int64(typ.Field(i).Offset), fieldType)
		}
	default:
		var mask []byte
		append2Slice(&mask, uintptr(unsafe.Pointer(typ.gcdata)), int(ptrData+7)/8)
		for i := int64(0); i < ptrData; i++ {
			if (mask[i/8]>>uint(i%8))&1 != 0 {
				w.Ptr(off/int64(linker.Arch.PtrSize) + i)
			}
		}
	}
}
