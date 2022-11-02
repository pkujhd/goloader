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
	typ  unsafe.Pointer
	word unsafe.Pointer
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

//go:linkname _methods reflect.(*uncommonType).methods
func _methods(t *uncommonType) []method

//go:linkname _Kind reflect.(*rtype).Kind
func _Kind(t *_type) reflect.Kind

func (t *_type) uncommon() *uncommonType    { return _uncommon(t) }
func (t *_type) nameOff(off nameOff) name   { return _nameOff(t, off) }
func (t *_type) typeOff(off typeOff) *_type { return _typeOff(t, off) }
func (n name) name() string                 { return _name(n) }
func (t *uncommonType) methods() []method   { return _methods(t) }
func (t *_type) Kind() reflect.Kind         { return _Kind(t) }

func pkgname(pkgpath string) string {
	slash := strings.LastIndexByte(pkgpath, '/')
	if slash > -1 {
		return pkgpath[slash+1:]
	} else {
		return pkgpath
	}
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
		pkgpath := (*_type)(header.typ).PkgPath()
		name := strings.Replace(v.Type().String(), pkgname(pkgpath), pkgpath, 1)
		symPtr[TypePrefix+name] = uintptr(header.typ)
	}

}
