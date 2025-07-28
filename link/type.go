package link

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/pkujhd/goloader/obj"
)

type tflag uint8

const (
	tflagUncommon      tflag = 1 << 0
	tflagExtraStar     tflag = 1 << 1
	tflagNamed         tflag = 1 << 2
	tflagRegularMemory tflag = 1 << 3 // equal and hash can treat values of this type as a single region of t.size bytes
)

// See reflect/value.go emptyInterface
type emptyInterface struct {
	typ  *_type
	word unsafe.Pointer
}

// See reflect/value.go sliceHeader
type sliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}

// See reflect/value.go stringHeader
type stringHeader struct {
	Data uintptr
	Len  int
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

type funcType struct {
	_type
	inCount  uint16
	outCount uint16 // top bit is set if last input parameter is ...
}

type uncommonType struct {
	pkgPath nameOff // import path; empty for built-in types like int, string
	mcount  uint16  // number of methods
	xcount  uint16  // number of exported methods
	moff    uint32  // offset from this uncommontype to [mcount]method
	_       uint32  // unused
}

func (t *uncommonType) methods() []method {
	if t.mcount == 0 {
		return nil
	}
	return (*[1 << 16]method)(add(unsafe.Pointer(t), uintptr(t.moff)))[:t.mcount:t.mcount]
}

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

func (t *_type) uncommon() *uncommonType         { return _uncommon(t) }
func (t *_type) nameOff(off nameOff) obj.Name    { return _nameOff(t, off) }
func (t *_type) typeOff(off typeOff) *_type      { return _typeOff(t, off) }
func (t *_type) Kind() reflect.Kind              { return _Kind(t) }
func (t *_type) NumField() int                   { return _NumField(t) }
func (t *_type) Field(i int) reflect.StructField { return _Field(t, i) }
func (t *_type) NumIn() int                      { return _NumIn(t) }
func (t *_type) In(i int) reflect.Type           { return _In(t, i) }
func (t *_type) NumOut() int                     { return _NumOut(t) }
func (t *_type) Out(i int) reflect.Type          { return _Out(t, i) }
func (t *_type) Key() reflect.Type               { return _Key(t) }
func (t *_type) Elem() reflect.Type              { return _Elem(t) }
func (t *_type) NumMethod() int                  { return _NumMethod(t) }
func (t *_type) Method(i int) reflect.Method     { return _Method(t, i) }
func (t *_type) ChanDir() reflect.ChanDir        { return _ChanDir(t) }
func (t *_type) Len() int                        { return _Len(t) }
func (t *_type) IsVariadic() bool                { return _IsVariadic(t) }
func (t *_type) Name() string                    { return _Name(t) }
func (t *_type) String() string                  { return _String(t) }
func (t *_type) PkgPath() string                 { return _PkgPath(t) }

func rtypeOf(i reflect.Type) *_type {
	eface := (*emptyInterface)(unsafe.Pointer(&i))
	return (*_type)(eface.word)
}

func resolveTypeName(typ *_type) string {
	pkgPath := obj.PathToPrefix(typ.PkgPath())
	name := typ.Name()
	if pkgPath != EmptyString && name != EmptyString {
		return pkgPath + "." + name
	}
	if name != EmptyString && typ.Kind() != reflect.UnsafePointer {
		return name
	}
	//golang <= 1.16 map.bucket has a self-contained struct field
	if strings.HasPrefix(typ.String(), "map.bucket[") {
		return typ.String()
	}
	switch typ.Kind() {
	case reflect.Ptr:
		name := "*" + resolveTypeName(rtypeOf(typ.Elem()))
		return name
	case reflect.Struct:
		if typ.NumField() == 0 {
			return typ.String()
		}
		fields := make([]string, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			fieldName := EmptyString
			if !typ.Field(i).Anonymous {
				if typ.Field(i).PkgPath != EmptyString {
					fieldName = obj.PathToPrefix(typ.Field(i).PkgPath) + "."
				}
				fieldName = fieldName + typ.Field(i).Name + " "
			}
			fields[i] = fieldName + resolveTypeName(rtypeOf(typ.Field(i).Type))
			if typ.Field(i).Tag != EmptyString {
				fields[i] = fields[i] + fmt.Sprintf(" %q", string(typ.Field(i).Tag))
			}
		}
		return fmt.Sprintf("struct { %s }", strings.Join(fields, "; "))
	case reflect.Map:
		return fmt.Sprintf("map[%s]%s", resolveTypeName(rtypeOf(typ.Key())), resolveTypeName(rtypeOf(typ.Elem())))
	case reflect.Chan:
		return fmt.Sprintf("%s %s", typ.ChanDir().String(), resolveTypeName(rtypeOf(typ.Elem())))
	case reflect.Slice:
		return fmt.Sprintf("[]%s", resolveTypeName(rtypeOf(typ.Elem())))
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", typ.Len(), resolveTypeName(rtypeOf(typ.Elem())))
	case reflect.Func:
		ins := make([]string, typ.NumIn())
		outs := make([]string, typ.NumOut())
		for i := 0; i < typ.NumIn(); i++ {
			if i == typ.NumIn()-1 && typ.IsVariadic() {
				ins[i] = "..." + resolveTypeName(rtypeOf(typ.In(i).Elem()))
			} else {
				ins[i] = resolveTypeName(rtypeOf(typ.In(i)))
			}
		}
		for i := 0; i < typ.NumOut(); i++ {
			outs[i] = resolveTypeName(rtypeOf(typ.Out(i)))
		}
		name := "func(" + strings.Join(ins, ", ") + ")"
		if len(outs) > 0 {
			name += " "
		}
		outName := strings.Join(outs, ", ")
		if len(outs) > 1 {
			outName = "(" + outName + ")"
		}
		return name + outName
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			return typ.String()
		}
		methods := make([]string, typ.NumMethod())
		for i := 0; i < typ.NumMethod(); i++ {
			method := typ.Method(i)
			methods[i] = method.Name + strings.TrimPrefix(resolveTypeName(rtypeOf(method.Type)), "func")
		}
		return fmt.Sprintf("interface { %s }", strings.Join(methods, "; "))
	case reflect.Bool,
		reflect.Int, reflect.Uint,
		reflect.Int64, reflect.Uint64,
		reflect.Int32, reflect.Uint32,
		reflect.Int16, reflect.Uint16,
		reflect.Int8, reflect.Uint8,
		reflect.Float64, reflect.Float32,
		reflect.Complex64, reflect.Complex128,
		reflect.String, reflect.UnsafePointer,
		reflect.Uintptr:
		return typ.String()
	default:
		panic("unexpected builtin type: " + typ.String())
	}
}

func RegTypes(symPtr map[string]uintptr, interfaces ...interface{}) {
	for _, inter := range interfaces {
		header := (*emptyInterface)(unsafe.Pointer(&inter))
		registerType(header.typ, symPtr)
	}
}
