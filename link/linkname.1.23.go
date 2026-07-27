//go:build go1.23 && !go1.28
// +build go1.23,!go1.28

package link

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/pkujhd/goloader/obj"
)

var (
	ptrSlice [64]uintptr
	ptrIndex = 0
)

var (
	lock         func(l *mutex)                           = nil
	unlock       func(l *mutex)                           = nil
	atomicstorep func(unsafe.Pointer, unsafe.Pointer)     = nil
	getitab      func(*interfacetype, *_type, bool) *itab = nil
)

var (
	resolveNameOff func(ptrInModule unsafe.Pointer, off int32) unsafe.Pointer
	resolveTypeOff func(rtype unsafe.Pointer, off int32) unsafe.Pointer = nil
)

func _nameOff(t *_type, off nameOff) obj.Name {
	return obj.Name{Bytes: (*byte)(resolveNameOff(unsafe.Pointer(t), int32(off)))}
}

func _typeOff(t *_type, off typeOff) *_type {
	return (*_type)(resolveTypeOff(unsafe.Pointer(t), int32(off)))
}

var (
	_uncommon   func(t *_type) *uncommonType              = nil
	_Kind       func(t *_type) reflect.Kind               = nil
	_NumField   func(t *_type) int                        = nil
	_Field      func(t *_type, i int) reflect.StructField = nil
	_NumIn      func(t *_type) int                        = nil
	_In         func(t *_type, i int) reflect.Type        = nil
	_NumOut     func(t *_type) int                        = nil
	_Out        func(t *_type, i int) reflect.Type        = nil
	_Key        func(t *_type) reflect.Type               = nil
	_Elem       func(t *_type) reflect.Type               = nil
	_NumMethod  func(t *_type) int                        = nil
	_Method     func(t *_type, i int) reflect.Method      = nil
	_ChanDir    func(t *_type) reflect.ChanDir            = nil
	_Len        func(t *_type) int                        = nil
	_IsVariadic func(t *_type) bool                       = nil
	_Name       func(t *_type) string                     = nil
	_String     func(t *_type) string                     = nil
	_PkgPath    func(t *_type) string                     = nil
)

var (
	typelinksinit     func()                                                                     = nil
	step              func(p []byte, pc *uintptr, val *int32, first bool) (newp []byte, ok bool) = nil
	findnull          func(s *byte) int                                                          = nil
	moduledataverify1 func(datap *moduledata)                                                    = nil
	modulesinit       func()                                                                     = nil
	progToPointerMask func(prog *byte, size uintptr) bitvector                                   = nil
)

var firstmoduledata *moduledata = nil

//go:inline
func getFuncPointer(symPtr map[string]uintptr, funcName string) uintptr {
	ptrIndex++
	ptrSlice[ptrIndex] = symPtr[funcName]
	if ptrSlice[ptrIndex] != 0 {
		return (uintptr)(unsafe.Pointer(&ptrSlice[ptrIndex]))
	}
	panic(fmt.Errorf("not found function:%s in runtime", funcName))
}

//go:inline
func getVarPointer(symPtr map[string]uintptr, name string) unsafe.Pointer {
	ptr := symPtr[name]
	if ptr != 0 {
		return unsafe.Pointer(ptr)
	}
	panic(fmt.Errorf("not found variable:%s in runtime", name))
}

func addLinkName(symPtr map[string]uintptr) {
	_Dummy(false)

	*(*uintptr)(unsafe.Pointer(&lock)) = getFuncPointer(symPtr, "runtime.lock")
	*(*uintptr)(unsafe.Pointer(&unlock)) = getFuncPointer(symPtr, "runtime.unlock")
	*(*uintptr)(unsafe.Pointer(&atomicstorep)) = getFuncPointer(symPtr, "internal/runtime/atomic.storePointer")
	*(*uintptr)(unsafe.Pointer(&getitab)) = getFuncPointer(symPtr, "runtime.getitab")

	*(*uintptr)(unsafe.Pointer(&resolveNameOff)) = getFuncPointer(symPtr, "runtime.resolveNameOff")
	*(*uintptr)(unsafe.Pointer(&resolveTypeOff)) = getFuncPointer(symPtr, "runtime.resolveTypeOff")

	*(*uintptr)(unsafe.Pointer(&_uncommon)) = getFuncPointer(symPtr, "internal/abi.(*Type).Uncommon")
	*(*uintptr)(unsafe.Pointer(&_Kind)) = getFuncPointer(symPtr, "reflect.(*rtype).Kind")
	*(*uintptr)(unsafe.Pointer(&_NumField)) = getFuncPointer(symPtr, "reflect.(*rtype).NumField")
	*(*uintptr)(unsafe.Pointer(&_Field)) = getFuncPointer(symPtr, "reflect.(*rtype).Field")
	*(*uintptr)(unsafe.Pointer(&_In)) = getFuncPointer(symPtr, "reflect.(*rtype).In")
	*(*uintptr)(unsafe.Pointer(&_NumIn)) = getFuncPointer(symPtr, "reflect.(*rtype).NumIn")
	*(*uintptr)(unsafe.Pointer(&_Out)) = getFuncPointer(symPtr, "reflect.(*rtype).Out")
	*(*uintptr)(unsafe.Pointer(&_NumOut)) = getFuncPointer(symPtr, "reflect.(*rtype).NumOut")
	*(*uintptr)(unsafe.Pointer(&_Key)) = getFuncPointer(symPtr, "reflect.(*rtype).Key")
	*(*uintptr)(unsafe.Pointer(&_Elem)) = getFuncPointer(symPtr, "reflect.(*rtype).Elem")
	*(*uintptr)(unsafe.Pointer(&_NumMethod)) = getFuncPointer(symPtr, "reflect.(*rtype).NumMethod")
	*(*uintptr)(unsafe.Pointer(&_Method)) = getFuncPointer(symPtr, "reflect.(*rtype).Method")
	*(*uintptr)(unsafe.Pointer(&_ChanDir)) = getFuncPointer(symPtr, "reflect.(*rtype).ChanDir")
	*(*uintptr)(unsafe.Pointer(&_Len)) = getFuncPointer(symPtr, "reflect.(*rtype).Len")
	*(*uintptr)(unsafe.Pointer(&_IsVariadic)) = getFuncPointer(symPtr, "reflect.(*rtype).IsVariadic")
	*(*uintptr)(unsafe.Pointer(&_Name)) = getFuncPointer(symPtr, "reflect.(*rtype).Name")
	*(*uintptr)(unsafe.Pointer(&_String)) = getFuncPointer(symPtr, "reflect.(*rtype).String")
	*(*uintptr)(unsafe.Pointer(&_PkgPath)) = getFuncPointer(symPtr, "reflect.(*rtype).PkgPath")

	*(*uintptr)(unsafe.Pointer(&typelinksinit)) = getFuncPointer(symPtr, "runtime.typelinksinit")
	*(*uintptr)(unsafe.Pointer(&step)) = getFuncPointer(symPtr, "runtime.step")
	*(*uintptr)(unsafe.Pointer(&findnull)) = getFuncPointer(symPtr, "runtime.findnull")
	*(*uintptr)(unsafe.Pointer(&moduledataverify1)) = getFuncPointer(symPtr, "runtime.moduledataverify1")
	*(*uintptr)(unsafe.Pointer(&modulesinit)) = getFuncPointer(symPtr, "runtime.modulesinit")
	*(*uintptr)(unsafe.Pointer(&progToPointerMask)) = getFuncPointer(symPtr, "runtime.progToPointerMask")

	firstmoduledata = (*moduledata)(getVarPointer(symPtr, "runtime.firstmoduledata"))

	addIfaceLinkName(symPtr)
	addInitLinkName(symPtr)
	addTypeLinkLinkName(symPtr)
	obj.AddLinkName(symPtr)
}

//go:noinline
func _Dummy(dummy bool) {
	//prevent the golang compiler from pruning reflect package symbols
	if dummy {
		channel := make(chan interface{}, 1)
		chanType := reflect.ValueOf(channel).Type()
		chanType.Elem().Method(0)
	}
}
