//go:build go1.21 && !go1.23
// +build go1.21,!go1.23

package goloader

import (
	"reflect"
	_ "unsafe"
)

//go:linkname _uncommon runtime.rtype.uncommon
func _uncommon(t *_type) *uncommonType

//go:linkname typesEqual runtime.typesEqual
func typesEqual(t, v *_type, seen map[_typePair]struct{}) bool

//go:linkname _nameOff runtime.rtype.nameOff
func _nameOff(t *_type, off nameOff) name

//go:linkname _typeOff runtime.rtype.typeOff
func _typeOff(t *_type, off typeOff) *_type

//go:linkname _name internal/abi.Name.Name
func _name(n name) string

//go:linkname _pkgPath runtime.pkgPath
func _pkgPath(n name) string

//go:linkname _isExported internal/abi.Name.IsExported
func _isExported(n name) bool

//go:linkname _methods internal/abi.(*UncommonType).Methods
func _methods(t *uncommonType) []method

//go:linkname _Kind reflect.(*rtype).Kind
func _Kind(t *_type) reflect.Kind

//go:linkname _Elem reflect.(*rtype).Elem
func _Elem(t *_type) reflect.Type
