package goloader

import (
	"reflect"
	"runtime"
	"strings"
	"unsafe"
)

type tflag uint8

// See runtime/type.go _typePair
type _typePair struct {
	t1 *_type
	t2 *_type
}

// See reflect/value.go emptyInterface
type emptyInterface struct {
	_type *_type
	data  unsafe.Pointer
}

func efaceOf(ep *interface{}) *emptyInterface {
	return (*emptyInterface)(unsafe.Pointer(ep))
}

// See reflect/value.go sliceHeader
type sliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}

// Method on non-interface type
type method struct {
	name nameOff // name of method
	mtyp typeOff // method type (without receiver)
	ifn  textOff // fn used in interface call (one-word receiver)
	tfn  textOff // fn used for normal method call
}

type imethod struct {
	name nameOff
	ityp typeOff
}

type interfacetype struct {
	typ     _type
	pkgpath name
	mhdr    []imethod
}

type name struct {
	bytes *byte
}

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
func _Elem(t *_type) *_type

func (t *_type) uncommon() *uncommonType    { return _uncommon(t) }
func (t *_type) nameOff(off nameOff) name   { return _nameOff(t, off) }
func (t *_type) typeOff(off typeOff) *_type { return _typeOff(t, off) }
func (n name) name() string                 { return _name(n) }
func (n name) pkgPath() string              { return _pkgPath(n) }
func (n name) isExported() bool             { return _isExported(n) }
func (t *uncommonType) methods() []method   { return _methods(t) }
func (t *_type) Kind() reflect.Kind         { return _Kind(t) }
func (t *_type) Elem() *_type               { return _Elem(t) }

// This replaces local package names with import paths, including where the package name doesn't match the last part of the import path e.g.
//
//	import "github.com/org/somepackage/v4" + somepackage.SomeStruct
//	 =>  github.com/org/somepackage/v4.SomeStruct
func fullyQualifiedName(t *_type, pkgpath string) string {
	// If pkgpath is empty, it's either:
	//  1) a builtin type
	//  2) an anonymous struct
	//  3) another anonymous composite type (e.g. array or slice)
	// For 2 and 3), we probably don't need to fully qualify the types as fully supporting cross-binary anonymous types will be awkward
	name := t.nameOff(t.str).name()
	if pkgpath == "" {
		return name
	}

	// Find the first dot, and read backwards until we find a '*' or a ']'
	dot := strings.IndexByte(name, '.')

	if dot == -1 {
		return name
	}
	start := dot
loop:
	for ; start >= 0; start-- {
		switch name[start] {
		case ']', '*', ' ':
			start++
			break loop
		}
	}
	localPkgName := name[start:dot]
	name = strings.Replace(name, localPkgName, pkgpath, 1)
	return name
}

func funcPkgPath(funcName string) string {
	// Anonymous struct methods can't have a package
	if strings.HasPrefix(funcName, "go.struct {") || strings.HasPrefix(funcName, "go.(*struct {") {
		return ""
	}
	lastSlash := strings.LastIndexByte(funcName, '/')
	if lastSlash == -1 {
		lastSlash = 0
	}
	// Methods on structs embedding structs from other packages look funny, e.g.:
	// regexp.(*onePassInst).regexp/syntax.op
	firstBracket := strings.LastIndex(funcName, ".(")
	if firstBracket > 0 && lastSlash > firstBracket {
		lastSlash = firstBracket
	}

	dot := lastSlash
	for ; dot < len(funcName) && funcName[dot] != '.' && funcName[dot] != '('; dot++ {
	}
	pkgPath := funcName[:dot]
	return strings.TrimPrefix(strings.TrimPrefix(pkgPath, "type..eq."), "[...]")
}

func (t *_type) PkgPath() string {
	ut := t.uncommon()
	if ut == nil {
		return EmptyString
	}
	return t.nameOff(ut.pkgpath).name()
}

func RegTypes(symPtr map[string]uintptr, interfaces ...interface{}) {
	for _, inter := range interfaces {
		v := reflect.ValueOf(inter)
		regType(symPtr, v)
		if v.Kind() == reflect.Ptr {
			regType(symPtr, v.Elem())
		}
	}
}

func regType(symPtr map[string]uintptr, v reflect.Value) {
	inter := v.Interface()
	if v.Kind() == reflect.Func && getFunctionPtr(inter) != 0 {
		symPtr[runtime.FuncForPC(v.Pointer()).Name()] = getFunctionPtr(inter)
	} else {
		header := (*emptyInterface)(unsafe.Pointer(&inter))
		t := header._type
		pkgpath := t.PkgPath()
		var element *_type
		var elementElem *_type
		if v.Kind() == reflect.Ptr {
			element = *(**_type)(add(unsafe.Pointer(t), unsafe.Sizeof(_type{})))
			if element != nil && pkgpath == EmptyString {
				switch element.Kind() {
				case reflect.Ptr, reflect.Array, reflect.Slice:
					elementElem = *(**_type)(add(unsafe.Pointer(element), unsafe.Sizeof(_type{})))
				}
				pkgpath = element.PkgPath()
				if elementElem != nil && pkgpath == EmptyString {
					pkgpath = elementElem.PkgPath()
				}
			}
		}
		name := fullyQualifiedName(t, pkgpath)
		if element != nil {
			symPtr[TypePrefix+name[1:]] = uintptr(unsafe.Pointer(element))
			if elementElem != nil {
				symPtr[TypePrefix+name[2:]] = uintptr(unsafe.Pointer(elementElem))
			}
		}
		symPtr[TypePrefix+name] = uintptr(unsafe.Pointer(header._type))
	}

}
