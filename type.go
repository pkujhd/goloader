package goloader

import (
	"reflect"
	"runtime"
	"strings"
	"unsafe"
)

type tflag uint8

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

//go:linkname (*_type).uncommon runtime.(*_type).uncommon
func (t *_type) uncommon() *uncommonType

//go:linkname (*_type).nameOff runtime.(*_type).nameOff
func (t *_type) nameOff(off nameOff) name

//go:linkname (*_type).typeOff runtime.(*_type).typeOff
func (t *_type) typeOff(off typeOff) *_type

//go:linkname name.name runtime.name.name
func (n name) name() string

//go:linkname getitab runtime.getitab
func getitab(inter *interfacetype, typ *_type, canfail bool) *itab

//go:linkname (*uncommonType).methods reflect.(*uncommonType).methods
func (t *uncommonType) methods() []method

func (t *_type) PkgPath() string {
	ut := t.uncommon()
	if ut == nil {
		return EMPTY_STRING
	}
	return t.nameOff(ut.pkgPath).name()
}

func RegTypes(symPtr map[string]uintptr, interfaces ...interface{}) {
	for _, inter := range interfaces {
		v := reflect.ValueOf(inter)
		registerTypeInfo(symPtr, v)
		if v.Kind() == reflect.Ptr {
			registerTypeInfo(symPtr, v.Elem())
		}
	}
}

func registerTypeInfo(symPtr map[string]uintptr, v reflect.Value) {
	inter := v.Interface()
	header := (*interfaceHeader)(unsafe.Pointer(&inter))

	if v.Kind() == reflect.Func && uintptr(header.word) != 0 {
		symPtr[runtime.FuncForPC(v.Pointer()).Name()] = *(*uintptr)(header.word)
	} else {
		name := TYPE_PREFIX
		symname := v.Type().String()
		if v.Type().Kind() == reflect.Ptr {
			name += symname[:1]
			symname = symname[1:]
		}
		pkgPath := (*_type)(header.typ).PkgPath()
		lastSlash := strings.LastIndexByte(pkgPath, '/')
		if lastSlash > -1 {
			name += pkgPath[:lastSlash+1] + symname
		} else {
			name += symname
		}
		symPtr[name] = uintptr(header.typ)
	}

}
