//go:build go1.18 && !go1.21
// +build go1.18,!go1.21

package goloader

import (
	"reflect"
	_ "unsafe"
)

//go:linkname _uncommon runtime.(*_type).uncommon
func _uncommon(t *_type) *uncommonType

//go:linkname typesEqual runtime.typesEqual
func typesEqual(t, v *_type, seen map[_typePair]struct{}) bool

//go:linkname _nameOff runtime.(*_type).nameOff
func _nameOff(t *_type, off nameOff) name

//go:linkname _typeOff runtime.(*_type).typeOff
func _typeOff(t *_type, off typeOff) *_type

//go:linkname _name runtime.name.name
func _name(n name) string

//go:linkname _pkgPath runtime.name.pkgPath
func _pkgPath(n name) string

//go:linkname _isExported runtime.name.isExported
func _isExported(n name) bool

//go:linkname _methods reflect.(*uncommonType).methods
func _methods(t *uncommonType) []method

//go:linkname _Kind reflect.(*rtype).Kind
func _Kind(t *_type) reflect.Kind

//go:linkname _Elem reflect.(*rtype).Elem
func _Elem(t *_type) reflect.Type
