package jsonunload

import (
	"reflect"
	"sync"
	"unsafe"
	_ "unsafe"
)

//go:linkname encoderCache encoding/json.encoderCache
var encoderCache sync.Map // map[reflect.Type]encoderFunc

type emptyInterface struct {
	typ  unsafe.Pointer
	word unsafe.Pointer
}

func uncacheTypes(dataStart, dataEnd uintptr) {
	encoderCache.Range(func(key, value any) bool {
		_, ok := key.(reflect.Type)
		if ok {
			eface := (*emptyInterface)(unsafe.Pointer(&key))
			typeAddr := uintptr(eface.word)
			if typeAddr > dataStart && typeAddr < dataEnd {
				encoderCache.Delete(key)
			}
		}
		return true
	})
}
