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
func (n name) name() (s string)

//go:linkname getitab runtime.getitab
func getitab(inter *interfacetype, typ *_type, canfail bool) *itab

func (t *_type) PkgPath() string {
	ut := t.uncommon()
	if ut == nil {
		return ""
	}
	return t.nameOff(ut.pkgPath).name()
}

func (t *_type) Name() string {
	return t.nameOff(t.str).name()
}

func (t *_type) Type() reflect.Type {
	var obj interface{} = reflect.TypeOf(0)
	(*interfaceHeader)(unsafe.Pointer(&obj)).word = unsafe.Pointer(t)
	typ := obj.(reflect.Type)
	return typ
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func RegTypes(symPtr map[string]uintptr, interfaces ...interface{}) {
	for _, ins := range interfaces {
		v := reflect.ValueOf(ins)
		regTypeInfo(symPtr, v)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
			regTypeInfo(symPtr, v)
		}
	}
}

func regTypeInfo(symPtr map[string]uintptr, v reflect.Value) {
	ins := v.Interface()
	header := (*interfaceHeader)(unsafe.Pointer(&ins))

	var ptr uintptr
	var typePrefix string
	var symName string
	pptr := (uintptr)(header.word)
	if v.Kind() == reflect.Func && pptr != 0 {
		ptr = *(*uintptr)(header.word)
		symName = GetFunctionName(ins)
	} else {
		ptr = uintptr(header.typ)
		typePrefix = "type."
		symName = v.Type().String()
	}

	if symName[0] == '*' {
		typePrefix += "*"
		symName = symName[1:]
	}

	pkgPath := (*_type)(header.typ).PkgPath()

	var symFullName string
	lastSlash := strings.LastIndexByte(pkgPath, '/')
	if lastSlash > -1 {
		symFullName = typePrefix + pkgPath[:lastSlash] + "/" + symName
	} else {
		symFullName = typePrefix + symName
	}
	symPtr[symFullName] = ptr
}
