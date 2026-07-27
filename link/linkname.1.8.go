//go:build go1.8 && !go1.23
// +build go1.8,!go1.23

package link

import (
	"reflect"
	"unsafe"

	_ "unsafe"

	"github.com/pkujhd/goloader/obj"
)

func addLinkName(symPtr map[string]uintptr) {

}

//go:linkname lock runtime.lock
func lock(l *mutex)

//go:linkname unlock runtime.unlock
func unlock(l *mutex)

//go:linkname atomicstorep runtime.atomicstorep
func atomicstorep(ptr unsafe.Pointer, new unsafe.Pointer)

//go:linkname getitab runtime.getitab
func getitab(inter *interfacetype, typ *_type, canfail bool) *itab

//go:linkname _nameOff reflect.(*rtype).nameOff
func _nameOff(t *_type, off nameOff) obj.Name

//go:linkname _typeOff reflect.(*rtype).typeOff
func _typeOff(t *_type, off typeOff) *_type

//go:linkname _uncommon reflect.(*rtype).uncommon
func _uncommon(t *_type) *uncommonType

//go:linkname _Kind reflect.(*rtype).Kind
func _Kind(t *_type) reflect.Kind

//go:linkname _NumField reflect.(*rtype).NumField
func _NumField(t *_type) int

//go:linkname _Field reflect.(*rtype).Field
func _Field(t *_type, i int) reflect.StructField

//go:linkname _NumIn reflect.(*rtype).NumIn
func _NumIn(t *_type) int

//go:linkname _In reflect.(*rtype).In
func _In(t *_type, i int) reflect.Type

//go:linkname _NumOut reflect.(*rtype).NumOut
func _NumOut(t *_type) int

//go:linkname _Out reflect.(*rtype).Out
func _Out(t *_type, i int) reflect.Type

//go:linkname _Key reflect.(*rtype).Key
func _Key(t *_type) reflect.Type

//go:linkname _Elem reflect.(*rtype).Elem
func _Elem(t *_type) reflect.Type

//go:linkname _NumMethod reflect.(*rtype).NumMethod
func _NumMethod(t *_type) int

//go:linkname _Method reflect.(*rtype).Method
func _Method(t *_type, i int) reflect.Method

//go:linkname _ChanDir reflect.(*rtype).ChanDir
func _ChanDir(t *_type) reflect.ChanDir

//go:linkname _Len reflect.(*rtype).Len
func _Len(t *_type) int

//go:linkname _IsVariadic reflect.(*rtype).IsVariadic
func _IsVariadic(t *_type) bool

//go:linkname _Name reflect.(*rtype).Name
func _Name(t *_type) string

//go:linkname _String reflect.(*rtype).String
func _String(t *_type) string

//go:linkname _PkgPath reflect.(*rtype).PkgPath
func _PkgPath(t *_type) string

//go:linkname typelinksinit runtime.typelinksinit
func typelinksinit()

//go:linkname step runtime.step
func step(p []byte, pc *uintptr, val *int32, first bool) (newp []byte, ok bool)

//go:linkname findnull runtime.findnull
func findnull(s *byte) int

//go:linkname moduledataverify1 runtime.moduledataverify1
func moduledataverify1(datap *moduledata)

//go:linkname modulesinit runtime.modulesinit
func modulesinit()

//go:linkname progToPointerMask runtime.progToPointerMask
func progToPointerMask(prog *byte, size uintptr) bitvector

//go:linkname _firstmoduledata runtime.firstmoduledata
var _firstmoduledata moduledata

var firstmoduledata *moduledata = &_firstmoduledata
