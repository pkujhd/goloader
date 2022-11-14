package reflectlite

import (
	"github.com/pkujhd/goloader/reflectlite/internal/unsafeheader"
	"strconv"
	"sync"
	"unsafe"
)

// Type is the representation of a Go type.
//
// Not all methods apply to all kinds of types. Restrictions,
// if any, are noted in the documentation for each method.
// Use the Kind method to find out the kind of type before
// calling kind-specific methods. Calling a method
// inappropriate to the kind of type causes a run-time panic.
//
// Type values are comparable, such as with the == operator,
// so they can be used as map keys.
// Two Type values are equal if they represent identical types.
type Type interface {

	// NumMethod returns the number of methods accessible using Method.
	//
	// For a non-interface type, it returns the number of exported methods.
	//
	// For an interface type, it returns the number of exported and unexported methods.
	NumMethod() int

	// Name returns the type's name within its package for a defined type.
	// For other (non-defined) types it returns the empty string.
	Name() string

	// PkgPath returns a defined type's package path, that is, the import path
	// that uniquely identifies the package, such as "encoding/base64".
	// If the type was predeclared (string, error) or not defined (*T, struct{},
	// []int, or A where A is an alias for a non-defined type), the package path
	// will be the empty string.
	PkgPath() string

	// String returns a string representation of the type.
	// The string representation may use shortened package names
	// (e.g., base64 instead of "encoding/base64") and is not
	// guaranteed to be unique among types. To test for type identity,
	// compare the Types directly.
	String() string

	// Kind returns the specific kind of this type.
	Kind() Kind

	// ConvertibleTo reports whether a value of the type is convertible to type u.
	// Even if ConvertibleTo returns true, the conversion may still panic.
	// For example, a slice of type []T is convertible to *[N]T,
	// but the conversion will panic if its length is less than N.
	ConvertibleTo(u Type) bool

	ConvertibleToWithInterface(u Type) bool

	// ChanDir returns a channel type's direction.
	// It panics if the type's Kind is not Chan.
	ChanDir() ChanDir

	// IsVariadic reports whether a function type's final input parameter
	// is a "..." parameter. If so, t.In(t.NumIn() - 1) returns the parameter's
	// implicit actual type []T.
	//
	// For concreteness, if t represents func(x int, y ... float64), then
	//
	//	t.NumIn() == 2
	//	t.In(0) is the reflect.Type for "int"
	//	t.In(1) is the reflect.Type for "[]float64"
	//	t.IsVariadic() == true
	//
	// IsVariadic panics if the type's Kind is not Func.
	IsVariadic() bool

	// Elem returns a type's element type.
	// It panics if the type's Kind is not Array, Chan, Map, Pointer, or Slice.
	Elem() Type

	// Field returns a struct type's i'th field.
	// It panics if the type's Kind is not Struct.
	// It panics if i is not in the range [0, NumField()).
	Field(i int) StructField

	// In returns the type of a function type's i'th input parameter.
	// It panics if the type's Kind is not Func.
	// It panics if i is not in the range [0, NumIn()).
	In(i int) Type

	// Key returns a map type's key type.
	// It panics if the type's Kind is not Map.
	Key() Type

	// Len returns an array type's length.
	// It panics if the type's Kind is not Array.
	Len() int

	// NumField returns a struct type's field count.
	// It panics if the type's Kind is not Struct.
	NumField() int

	// NumIn returns a function type's input parameter count.
	// It panics if the type's Kind is not Func.
	NumIn() int

	// NumOut returns a function type's output parameter count.
	// It panics if the type's Kind is not Func.
	NumOut() int

	// Out returns the type of a function type's i'th output parameter.
	// It panics if the type's Kind is not Func.
	// It panics if i is not in the range [0, NumOut()).
	Out(i int) Type

	common() *rtype
	uncommon() *uncommonType
}

var _ Type = (*rtype)(nil)

// A Kind represents the specific kind of type that a Type represents.
// The zero Kind is not a valid kind.
type Kind uint

const (
	Invalid Kind = iota
	Bool
	Int
	Int8
	Int16
	Int32
	Int64
	Uint
	Uint8
	Uint16
	Uint32
	Uint64
	Uintptr
	Float32
	Float64
	Complex64
	Complex128
	Array
	Chan
	Func
	Interface
	Map
	Pointer
	Slice
	String
	Struct
	UnsafePointer
)

// Ptr is the old name for the Pointer kind.
const Ptr = Pointer

var kindNames = []string{
	Invalid:       "invalid",
	Bool:          "bool",
	Int:           "int",
	Int8:          "int8",
	Int16:         "int16",
	Int32:         "int32",
	Int64:         "int64",
	Uint:          "uint",
	Uint8:         "uint8",
	Uint16:        "uint16",
	Uint32:        "uint32",
	Uint64:        "uint64",
	Uintptr:       "uintptr",
	Float32:       "float32",
	Float64:       "float64",
	Complex64:     "complex64",
	Complex128:    "complex128",
	Array:         "array",
	Chan:          "chan",
	Func:          "func",
	Interface:     "interface",
	Map:           "map",
	Pointer:       "ptr",
	Slice:         "slice",
	String:        "string",
	Struct:        "struct",
	UnsafePointer: "unsafe.Pointer",
}

// String returns the name of k.
func (k Kind) String() string {
	if uint(k) < uint(len(kindNames)) {
		return kindNames[uint(k)]
	}
	return "kind" + strconv.Itoa(int(k))
}

// tflag is used by an rtype to signal what extra type information is
// available in the memory directly following the rtype value.
//
// tflag values must be kept in sync with copies in:
//
//	cmd/compile/internal/reflectdata/reflect.go
//	cmd/link/internal/ld/decodesym.go
//	runtime/type.go
type tflag uint8

const (
	// tflagUncommon means that there is a pointer, *uncommonType,
	// just beyond the outer type structure.
	//
	// For example, if t.Kind() == Struct and t.tflag&tflagUncommon != 0,
	// then t has uncommonType data and it can be accessed as:
	//
	//	type tUncommon struct {
	//		structType
	//		u uncommonType
	//	}
	//	u := &(*tUncommon)(unsafe.Pointer(t)).u
	tflagUncommon tflag = 1 << 0

	// tflagExtraStar means the name in the str field has an
	// extraneous '*' prefix. This is because for most types T in
	// a program, the type *T also exists and reusing the str data
	// saves binary size.
	tflagExtraStar tflag = 1 << 1

	// tflagNamed means the type has a name.
	tflagNamed tflag = 1 << 2

	// tflagRegularMemory means that equal and hash functions can treat
	// this type as a single region of t.size bytes.
	tflagRegularMemory tflag = 1 << 3
)

const (
	kindDirectIface = 1 << 5
	kindGCProg      = 1 << 6 // Type.gc points to GC program
	kindMask        = (1 << 5) - 1
)

// resolveNameOff resolves a name offset from a base pointer.
// The (*rtype).nameOff method is a convenience wrapper for this function.
// Implemented in the runtime package.
//
//go:linkname resolveNameOff reflect.resolveNameOff
func resolveNameOff(ptrInModule unsafe.Pointer, off int32) unsafe.Pointer

// resolveTypeOff resolves an *rtype offset from a base type.
// The (*rtype).typeOff method is a convenience wrapper for this function.
// Implemented in the runtime package.
//
//go:linkname resolveTypeOff reflect.resolveTypeOff
func resolveTypeOff(rtype unsafe.Pointer, off int32) unsafe.Pointer

// resolveTextOff resolves a function pointer offset from a base type.
// The (*rtype).textOff method is a convenience wrapper for this function.
// Implemented in the runtime package.
//
//go:linkname resolveTextOff reflect.resolveTextOff
func resolveTextOff(rtype unsafe.Pointer, off int32) unsafe.Pointer

//go:linkname addReflectOff reflect.addReflectOff
func addReflectOff(ptr unsafe.Pointer) int32

// resolveReflectName adds a name to the reflection lookup map in the runtime.
// It returns a new nameOff that can be used to refer to the pointer.
func resolveReflectName(n name) nameOff {
	return nameOff(addReflectOff(unsafe.Pointer(n.bytes)))
}

// resolveReflectType adds a *rtype to the reflection lookup map in the runtime.
// It returns a new typeOff that can be used to refer to the pointer.
func resolveReflectType(t *rtype) typeOff {
	return typeOff(addReflectOff(unsafe.Pointer(t)))
}

// resolveReflectText adds a function pointer to the reflection lookup map in
// the runtime. It returns a new textOff that can be used to refer to the
// pointer.
func resolveReflectText(ptr unsafe.Pointer) textOff {
	return textOff(addReflectOff(ptr))
}

type nameOff int32 // offset to a name
type typeOff int32 // offset to an *rtype
type textOff int32 // offset from top of text section

func (t *rtype) nameOff(off nameOff) name {
	return name{(*byte)(resolveNameOff(unsafe.Pointer(t), int32(off)))}
}

func (t *rtype) typeOff(off typeOff) *rtype {
	return (*rtype)(resolveTypeOff(unsafe.Pointer(t), int32(off)))
}

func (t *rtype) textOff(off textOff) unsafe.Pointer {
	return resolveTextOff(unsafe.Pointer(t), int32(off))
}

// rtype is the common implementation of most values.
// It is embedded in other struct types.
//
// rtype must be kept in sync with ../runtime/type.go:/^type._type.
type rtype struct {
	size       uintptr
	ptrdata    uintptr // number of bytes in the type that can contain pointers
	hash       uint32  // hash of type; avoids computation in hash tables
	tflag      tflag   // extra type information flags
	align      uint8   // alignment of variable with this type
	fieldAlign uint8   // alignment of struct field with this type
	kind       uint8   // enumeration for C
	// function for comparing objects of this type
	// (ptr to object A, ptr to object B) -> ==?
	equal     func(unsafe.Pointer, unsafe.Pointer) bool
	gcdata    *byte   // garbage collection data
	str       nameOff // string form
	ptrToThis typeOff // type for pointer to this type, may be zero
}

func (t *rtype) hasName() bool {
	return t.tflag&tflagNamed != 0
}

func (t *rtype) Name() string {
	if !t.hasName() {
		return ""
	}
	s := t.String()
	i := len(s) - 1
	sqBrackets := 0
	for i >= 0 && (s[i] != '.' || sqBrackets != 0) {
		switch s[i] {
		case ']':
			sqBrackets++
		case '[':
			sqBrackets--
		}
		i--
	}
	return s[i+1:]
}

//go:linkname typelinks reflect.typelinks
func typelinks() (sections []unsafe.Pointer, offset [][]int32)

func rtypeOff(section unsafe.Pointer, off int32) *rtype {
	return (*rtype)(add(section, uintptr(off), "sizeof(rtype) > 0"))
}

// typesByString returns the subslice of typelinks() whose elements have
// the given string representation.
// It may be empty (no known types with that string) or may have
// multiple elements (multiple types with that string).
func typesByString(s string) []*rtype {
	sections, offset := typelinks()
	var ret []*rtype

	for offsI, offs := range offset {
		section := sections[offsI]

		// We are looking for the first index i where the string becomes >= s.
		// This is a copy of sort.Search, with f(h) replaced by (*typ[h].String() >= s).
		i, j := 0, len(offs)
		for i < j {
			h := i + (j-i)>>1 // avoid overflow when computing h
			// i ≤ h < j
			if !(rtypeOff(section, offs[h]).String() >= s) {
				i = h + 1 // preserves f(i-1) == false
			} else {
				j = h // preserves f(j) == true
			}
		}
		// i == j, f(i-1) == false, and f(j) (= f(i)) == true  =>  answer is i.

		// Having found the first, linear scan forward to find the last.
		// We could do a second binary search, but the caller is going
		// to do a linear scan anyway.
		for j := i; j < len(offs); j++ {
			typ := rtypeOff(section, offs[j])
			if typ.String() != s {
				break
			}
			ret = append(ret, typ)
		}
	}
	return ret
}

// TypeOf returns the reflection Type that represents the dynamic type of i.
// If i is a nil interface value, TypeOf returns nil.
func TypeOf(i interface{}) Type {
	eface := *(*emptyInterface)(unsafe.Pointer(&i))
	return toType(eface.typ)
}

// toType converts from a *rtype to a Type that can be returned
// to the client of package reflect. In gc, the only concern is that
// a nil *rtype must be replaced by a nil Type, but in gccgo this
// function takes care of ensuring that multiple *rtype for the same
// type are coalesced into a single Type.
func toType(t *rtype) Type {
	if t == nil {
		return nil
	}
	return t
}

// ptrMap is the cache for PointerTo.
var ptrMap sync.Map // map[*rtype]*ptrType

func (t *rtype) ptrTo() *rtype {
	if t.ptrToThis != 0 {
		return t.typeOff(t.ptrToThis)
	}

	// Check the cache.
	if pi, ok := ptrMap.Load(t); ok {
		return &pi.(*ptrType).rtype
	}

	// Look in known types.
	s := "*" + t.String()
	for _, tt := range typesByString(s) {
		p := (*ptrType)(unsafe.Pointer(tt))
		if p.elem != t {
			continue
		}
		pi, _ := ptrMap.LoadOrStore(t, p)
		return &pi.(*ptrType).rtype
	}

	// Create a new ptrType starting with the description
	// of an *unsafe.Pointer.
	var iptr interface{} = (*unsafe.Pointer)(nil)
	prototype := *(**ptrType)(unsafe.Pointer(&iptr))
	pp := *prototype

	pp.str = resolveReflectName(newName(s, "", false, false))
	pp.ptrToThis = 0

	// For the type structures linked into the binary, the
	// compiler provides a good hash of the string.
	// Create a good hash for the new string by using
	// the FNV-1 hash's mixing function to combine the
	// old hash and the new "*".
	pp.hash = fnv1(t.hash, '*')

	pp.elem = t

	pi, _ := ptrMap.LoadOrStore(t, &pp)
	return &pi.(*ptrType).rtype
}

// fnv1 incorporates the list of bytes into the hash x using the FNV-1 hash function.
func fnv1(x uint32, list ...byte) uint32 {
	for _, b := range list {
		x = x*16777619 ^ uint32(b)
	}
	return x
}

func (t *rtype) PkgPath() string {
	if t.tflag&tflagNamed == 0 {
		return ""
	}
	ut := t.uncommon()
	if ut == nil {
		return ""
	}
	return t.nameOff(ut.pkgPath).name()
}

func (t *rtype) String() string {
	s := t.nameOff(t.str).name()
	if t.tflag&tflagExtraStar != 0 {
		return s[1:]
	}
	return s
}

func (t *rtype) ConvertibleTo(u Type) bool {
	if u == nil {
		panic("reflect: nil type passed to Type.ConvertibleTo")
	}
	uu := u.(*rtype)
	return convertOp(uu, t, false) != nil
}

func (t *rtype) ConvertibleToWithInterface(u Type) bool {
	if u == nil {
		panic("reflect: nil type passed to Type.ConvertibleTo")
	}
	uu := u.(*rtype)
	return convertOp(uu, t, true) != nil
}

// implements reports whether the type V implements the interface type T.
func implements(T, V *rtype) bool {
	if T.Kind() != Interface {
		return false
	}
	t := (*interfaceType)(unsafe.Pointer(T))
	if len(t.methods) == 0 {
		return true
	}

	// The same algorithm applies in both cases, but the
	// method tables for an interface type and a concrete type
	// are different, so the code is duplicated.
	// In both cases the algorithm is a linear scan over the two
	// lists - T's methods and V's methods - simultaneously.
	// Since method tables are stored in a unique sorted order
	// (alphabetical, with no duplicate method names), the scan
	// through V's methods must hit a match for each of T's
	// methods along the way, or else V does not implement T.
	// This lets us run the scan in overall linear time instead of
	// the quadratic time  a naive search would require.
	// See also ../runtime/iface.go.
	if V.Kind() == Interface {
		v := (*interfaceType)(unsafe.Pointer(V))
		i := 0
		for j := 0; j < len(v.methods); j++ {
			tm := &t.methods[i]
			tmName := t.nameOff(tm.name)
			vm := &v.methods[j]
			vmName := V.nameOff(vm.name)
			if vmName.name() == tmName.name() && haveIdenticalUnderlyingType(V.typeOff(vm.typ), t.typeOff(tm.typ), false, true) {
				if !tmName.isExported() {
					tmPkgPath := tmName.pkgPath()
					if tmPkgPath == "" {
						tmPkgPath = t.pkgPath.name()
					}
					vmPkgPath := vmName.pkgPath()
					if vmPkgPath == "" {
						vmPkgPath = v.pkgPath.name()
					}
					if tmPkgPath != vmPkgPath {
						continue
					}
				}
				if i++; i >= len(t.methods) {
					return true
				}
			}
		}
		return false
	}

	v := V.uncommon()
	if v == nil {
		return false
	}
	i := 0
	vmethods := v.methods()
	for j := 0; j < int(v.mcount); j++ {
		tm := &t.methods[i]
		tmName := t.nameOff(tm.name)
		vm := vmethods[j]
		vmName := V.nameOff(vm.name)
		if vmName.name() == tmName.name() && V.typeOff(vm.mtyp) == t.typeOff(tm.typ) {
			if !tmName.isExported() {
				tmPkgPath := tmName.pkgPath()
				if tmPkgPath == "" {
					tmPkgPath = t.pkgPath.name()
				}
				vmPkgPath := vmName.pkgPath()
				if vmPkgPath == "" {
					vmPkgPath = V.nameOff(v.pkgPath).name()
				}
				if tmPkgPath != vmPkgPath {
					continue
				}
			}
			if i++; i >= len(t.methods) {
				return true
			}
		}
	}
	return false
}

// specialChannelAssignability reports whether a value x of channel type V
// can be directly assigned (using memmove) to another channel type T.
// https://golang.org/doc/go_spec.html#Assignability
// T and V must be both of Chan kind.
func specialChannelAssignability(T, V *rtype) bool {
	// Special case:
	// x is a bidirectional channel value, T is a channel type,
	// x's type V and T have identical element types,
	// and at least one of V or T is not a defined type.
	return V.ChanDir() == BothDir && (T.Name() == "" || V.Name() == "") && haveIdenticalType(T.Elem(), V.Elem(), true, true)
}

func (t *rtype) ChanDir() ChanDir {
	if t.Kind() != Chan {
		panic("reflect: ChanDir of non-chan type " + t.String())
	}
	tt := (*chanType)(unsafe.Pointer(t))
	return ChanDir(tt.dir)
}

func (t *rtype) IsVariadic() bool {
	if t.Kind() != Func {
		panic("reflect: IsVariadic of non-func type " + t.String())
	}
	tt := (*funcType)(unsafe.Pointer(t))
	return tt.outCount&(1<<15) != 0
}

func (t *rtype) Elem() Type {
	switch t.Kind() {
	case Array:
		tt := (*arrayType)(unsafe.Pointer(t))
		return toType(tt.elem)
	case Chan:
		tt := (*chanType)(unsafe.Pointer(t))
		return toType(tt.elem)
	case Map:
		tt := (*mapType)(unsafe.Pointer(t))
		return toType(tt.elem)
	case Pointer:
		tt := (*ptrType)(unsafe.Pointer(t))
		return toType(tt.elem)
	case Slice:
		tt := (*sliceType)(unsafe.Pointer(t))
		return toType(tt.elem)
	}
	panic("reflect: Elem of invalid type " + t.String())
}

func (t *rtype) Field(i int) StructField {
	if t.Kind() != Struct {
		panic("reflect: Field of non-struct type " + t.String())
	}
	tt := (*structType)(unsafe.Pointer(t))
	return tt.Field(i)
}

func (t *rtype) In(i int) Type {
	if t.Kind() != Func {
		panic("reflect: In of non-func type " + t.String())
	}
	tt := (*funcType)(unsafe.Pointer(t))
	return toType(tt.in()[i])
}

func (t *rtype) Key() Type {
	if t.Kind() != Map {
		panic("reflect: Key of non-map type " + t.String())
	}
	tt := (*mapType)(unsafe.Pointer(t))
	return toType(tt.key)
}

func (t *rtype) Len() int {
	if t.Kind() != Array {
		panic("reflect: Len of non-array type " + t.String())
	}
	tt := (*arrayType)(unsafe.Pointer(t))
	return int(tt.len)
}

func (t *rtype) NumField() int {
	if t.Kind() != Struct {
		panic("reflect: NumField of non-struct type " + t.String())
	}
	tt := (*structType)(unsafe.Pointer(t))
	return len(tt.fields)
}

func (t *rtype) NumIn() int {
	if t.Kind() != Func {
		panic("reflect: NumIn of non-func type " + t.String())
	}
	tt := (*funcType)(unsafe.Pointer(t))
	return int(tt.inCount)
}

func (t *rtype) NumOut() int {
	if t.Kind() != Func {
		panic("reflect: NumOut of non-func type " + t.String())
	}
	tt := (*funcType)(unsafe.Pointer(t))
	return len(tt.out())
}

func (t *rtype) Out(i int) Type {
	if t.Kind() != Func {
		panic("reflect: Out of non-func type " + t.String())
	}
	tt := (*funcType)(unsafe.Pointer(t))
	return toType(tt.out()[i])
}

func (t *funcType) in() []*rtype {
	uadd := unsafe.Sizeof(*t)
	if t.tflag&tflagUncommon != 0 {
		uadd += unsafe.Sizeof(uncommonType{})
	}
	if t.inCount == 0 {
		return nil
	}
	return (*[1 << 20]*rtype)(add(unsafe.Pointer(t), uadd, "t.inCount > 0"))[:t.inCount:t.inCount]
}

func (t *funcType) out() []*rtype {
	uadd := unsafe.Sizeof(*t)
	if t.tflag&tflagUncommon != 0 {
		uadd += unsafe.Sizeof(uncommonType{})
	}
	outCount := t.outCount & (1<<15 - 1)
	if outCount == 0 {
		return nil
	}
	return (*[1 << 20]*rtype)(add(unsafe.Pointer(t), uadd, "outCount > 0"))[t.inCount : t.inCount+outCount : t.inCount+outCount]
}

func (t *rtype) common() *rtype { return t }

// Method on non-interface type
type method struct {
	name nameOff // name of method
	mtyp typeOff // method type (without receiver)
	ifn  textOff // fn used in interface call (one-word receiver)
	tfn  textOff // fn used for normal method call
}

// uncommonType is present only for defined types or types with methods
// (if T is a defined type, the uncommonTypes for T and *T have methods).
// Using a pointer to this struct reduces the overall size required
// to describe a non-defined type with no methods.
type uncommonType struct {
	pkgPath nameOff // import path; empty for built-in types like int, string
	mcount  uint16  // number of methods
	xcount  uint16  // number of exported methods
	moff    uint32  // offset from this uncommontype to [mcount]method
	_       uint32  // unused
}

// ChanDir represents a channel type's direction.
type ChanDir int

const (
	RecvDir ChanDir             = 1 << iota // <-chan
	SendDir                                 // chan<-
	BothDir = RecvDir | SendDir             // chan
)

// arrayType represents a fixed array type.
type arrayType struct {
	rtype
	elem  *rtype // array element type
	slice *rtype // slice type
	len   uintptr
}

// chanType represents a channel type.
type chanType struct {
	rtype
	elem *rtype  // channel element type
	dir  uintptr // channel direction (ChanDir)
}

// funcType represents a function type.
//
// A *rtype for each in and out parameter is stored in an array that
// directly follows the funcType (and possibly its uncommonType). So
// a function type with one method, one input, and one output is:
//
//	struct {
//		funcType
//		uncommonType
//		[2]*rtype    // [0] is in, [1] is out
//	}
type funcType struct {
	rtype
	inCount  uint16
	outCount uint16 // top bit is set if last input parameter is ...
}

// imethod represents a method on an interface type
type imethod struct {
	name nameOff // name of method
	typ  typeOff // .(*FuncType) underneath
}

// interfaceType represents an interface type.
type interfaceType struct {
	rtype
	pkgPath name      // import path
	methods []imethod // sorted by hash
}

// mapType represents a map type.
type mapType struct {
	rtype
	key    *rtype // map key type
	elem   *rtype // map element (value) type
	bucket *rtype // internal bucket structure
	// function for hashing keys (ptr to key, seed) -> hash
	hasher     func(unsafe.Pointer, uintptr) uintptr
	keysize    uint8  // size of key slot
	valuesize  uint8  // size of value slot
	bucketsize uint16 // size of bucket
	flags      uint32
}

// ptrType represents a pointer type.
type ptrType struct {
	rtype
	elem *rtype // pointer element (pointed at) type
}

// sliceType represents a slice type.
type sliceType struct {
	rtype
	elem *rtype // slice element type
}

// Struct field
type structField struct {
	name   name    // name is always non-empty
	typ    *rtype  // type of field
	offset uintptr // byte offset of field
}

func (f *structField) embedded() bool {
	return f.name.embedded()
}

// structType represents a struct type.
type structType struct {
	rtype
	pkgPath name
	fields  []structField // sorted by offset
}

type structTypeUncommon struct {
	structType
	u uncommonType
}

// Field returns the i'th struct field.
func (t *structType) Field(i int) (f StructField) {
	if i < 0 || i >= len(t.fields) {
		panic("reflect: Field index out of bounds")
	}
	p := &t.fields[i]
	f.Type = toType(p.typ)
	f.Name = p.name.name()
	f.Anonymous = p.embedded()
	if !p.name.isExported() {
		f.PkgPath = t.pkgPath.name()
	}
	if tag := p.name.tag(); tag != "" {
		f.Tag = StructTag(tag)
	}
	f.Offset = p.offset

	// NOTE(rsc): This is the only allocation in the interface
	// presented by a reflect.Type. It would be nice to avoid,
	// at least in the common cases, but we need to make sure
	// that misbehaving clients of reflect cannot affect other
	// uses of reflect. One possibility is CL 5371098, but we
	// postponed that ugliness until there is a demonstrated
	// need for the performance. This is issue 2320.
	f.Index = []int{i}
	return
}

// name is an encoded type name with optional extra data.
//
// The first byte is a bit field containing:
//
//	1<<0 the name is exported
//	1<<1 tag data follows the name
//	1<<2 pkgPath nameOff follows the name and tag
//	1<<3 the name is of an embedded (a.k.a. anonymous) field
//
// Following that, there is a varint-encoded length of the name,
// followed by the name itself.
//
// If tag data is present, it also has a varint-encoded length
// followed by the tag itself.
//
// If the import path follows, then 4 bytes at the end of
// the data form a nameOff. The import path is only set for concrete
// methods that are defined in a different package than their type.
//
// If a name starts with "*", then the exported bit represents
// whether the pointed to type is exported.
//
// Note: this encoding must match here and in:
//   cmd/compile/internal/reflectdata/reflect.go
//   runtime/type.go
//   internal/reflectlite/type.go
//   cmd/link/internal/ld/decodesym.go

type name struct {
	bytes *byte
}

func (n name) data(off int, whySafe string) *byte {
	return (*byte)(add(unsafe.Pointer(n.bytes), uintptr(off), whySafe))
}

func (n name) isExported() bool {
	return (*n.bytes)&(1<<0) != 0
}

func (n name) hasTag() bool {
	return (*n.bytes)&(1<<1) != 0
}

func (n name) embedded() bool {
	return (*n.bytes)&(1<<3) != 0
}

func (n name) name() (s string) {
	if n.bytes == nil {
		return
	}
	i, l := n.readVarint(1)
	hdr := (*unsafeheader.String)(unsafe.Pointer(&s))
	hdr.Data = unsafe.Pointer(n.data(1+i, "non-empty string"))
	hdr.Len = l
	return
}

func (n name) tag() (s string) {
	if !n.hasTag() {
		return ""
	}
	i, l := n.readVarint(1)
	i2, l2 := n.readVarint(1 + i + l)
	hdr := (*unsafeheader.String)(unsafe.Pointer(&s))
	hdr.Data = unsafe.Pointer(n.data(1+i+l+i2, "non-empty string"))
	hdr.Len = l2
	return
}

func (n name) pkgPath() string {
	if n.bytes == nil || *n.data(0, "name flag field")&(1<<2) == 0 {
		return ""
	}
	i, l := n.readVarint(1)
	off := 1 + i + l
	if n.hasTag() {
		i2, l2 := n.readVarint(off)
		off += i2 + l2
	}
	var nameOff int32
	// Note that this field may not be aligned in memory,
	// so we cannot use a direct int32 assignment here.
	copy((*[4]byte)(unsafe.Pointer(&nameOff))[:], (*[4]byte)(unsafe.Pointer(n.data(off, "name offset field")))[:])
	pkgPathName := name{(*byte)(resolveTypeOff(unsafe.Pointer(n.bytes), nameOff))}
	return pkgPathName.name()
}

func newName(n, tag string, exported, embedded bool) name {
	if len(n) >= 1<<29 {
		panic("reflect.nameFrom: name too long: " + n[:1024] + "...")
	}
	if len(tag) >= 1<<29 {
		panic("reflect.nameFrom: tag too long: " + tag[:1024] + "...")
	}
	var nameLen [10]byte
	var tagLen [10]byte
	nameLenLen := writeVarint(nameLen[:], len(n))
	tagLenLen := writeVarint(tagLen[:], len(tag))

	var bits byte
	l := 1 + nameLenLen + len(n)
	if exported {
		bits |= 1 << 0
	}
	if len(tag) > 0 {
		l += tagLenLen + len(tag)
		bits |= 1 << 1
	}
	if embedded {
		bits |= 1 << 3
	}

	b := make([]byte, l)
	b[0] = bits
	copy(b[1:], nameLen[:nameLenLen])
	copy(b[1+nameLenLen:], n)
	if len(tag) > 0 {
		tb := b[1+nameLenLen+len(n):]
		copy(tb, tagLen[:tagLenLen])
		copy(tb[tagLenLen:], tag)
	}

	return name{bytes: &b[0]}
}

type StructTag string

// A StructField describes a single field in a struct.
type StructField struct {
	// Name is the field name.
	Name string

	// PkgPath is the package path that qualifies a lower case (unexported)
	// field name. It is empty for upper case (exported) field names.
	// See https://golang.org/ref/spec#Uniqueness_of_identifiers
	PkgPath string

	Type      Type      // field type
	Tag       StructTag // field tag string
	Offset    uintptr   // offset within struct, in bytes
	Index     []int     // index sequence for Type.FieldByIndex
	Anonymous bool      // is an embedded field
}

// readVarint parses a varint as encoded by encoding/binary.
// It returns the number of encoded bytes and the encoded value.
func (n name) readVarint(off int) (int, int) {
	v := 0
	for i := uint(0); ; i++ {
		x := *n.data(off+int(i), "read varint")
		v += int(x&0x7f) << (7 * i)
		if x&0x80 == 0 {
			return int(i + 1), v
		}
	}
}

// writeVarint writes n to buf in varint form. Returns the
// number of bytes written. n must be nonnegative.
// Writes at most 10 bytes.
func writeVarint(buf []byte, n int) int {
	for i := 0; ; i++ {
		b := byte(n & 0x7f)
		n >>= 7
		if n == 0 {
			buf[i] = b
			return i + 1
		}
		buf[i] = b | 0x80
	}
}

// add returns p+x.
//
// The whySafe string is ignored, so that the function still inlines
// as efficiently as p+x, but all call sites should use the string to
// record why the addition is safe, which is to say why the addition
// does not cause x to advance to the very end of p's allocation
// and therefore point incorrectly at the next block in memory.
func add(p unsafe.Pointer, x uintptr, whySafe string) unsafe.Pointer {
	return unsafe.Pointer(uintptr(p) + x)
}

func (t *rtype) pointers() bool { return t.ptrdata != 0 }

func (t *rtype) Kind() Kind { return Kind(t.kind & kindMask) }

func (t *rtype) NumMethod() int {
	if t.Kind() == Interface {
		tt := (*interfaceType)(unsafe.Pointer(t))
		return tt.NumMethod()
	}
	return len(t.exportedMethods())
}

func (t *rtype) uncommon() *uncommonType {
	if t.tflag&tflagUncommon == 0 {
		return nil
	}
	switch t.Kind() {
	case Struct:
		return &(*structTypeUncommon)(unsafe.Pointer(t)).u
	case Pointer:
		type u struct {
			ptrType
			u uncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Func:
		type u struct {
			funcType
			u uncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Slice:
		type u struct {
			sliceType
			u uncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Array:
		type u struct {
			arrayType
			u uncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Chan:
		type u struct {
			chanType
			u uncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Map:
		type u struct {
			mapType
			u uncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	case Interface:
		type u struct {
			interfaceType
			u uncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	default:
		type u struct {
			rtype
			u uncommonType
		}
		return &(*u)(unsafe.Pointer(t)).u
	}
}

func (t *rtype) exportedMethods() []method {
	ut := t.uncommon()
	if ut == nil {
		return nil
	}
	return ut.exportedMethods()
}

func (t *uncommonType) methods() []method {
	if t.mcount == 0 {
		return nil
	}
	return (*[1 << 16]method)(add(unsafe.Pointer(t), uintptr(t.moff), "t.mcount > 0"))[:t.mcount:t.mcount]
}

func (t *uncommonType) exportedMethods() []method {
	if t.xcount == 0 {
		return nil
	}
	return (*[1 << 16]method)(add(unsafe.Pointer(t), uintptr(t.moff), "t.xcount > 0"))[:t.xcount:t.xcount]
}

// NumMethod returns the number of interface methods in the type's method set.
func (t *interfaceType) NumMethod() int { return len(t.methods) }
