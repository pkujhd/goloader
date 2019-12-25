package goloader

import (
	"cmd/objfile/objfile"
	"encoding/binary"
	"os"
	"reflect"
	"strings"
	"unsafe"
)

const (
	TLSNAME        = "(TLS)"
	R_CALLIND_NAME = "R_CALLIND"
)

// See reflect/value.go emptyInterface
type interfaceHeader struct {
	typ  uintptr
	word unsafe.Pointer
}

// See reflect/value.go stringHeader
type stringHeader struct {
	Data uintptr
	Len  int
}

// See reflect/value.go sliceHeader
type sliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}

// RegSymbol register common types for relocation
func regBasicSymbol(symPtr map[string]uintptr) {
	int_0 := int(0)
	int8_0 := int8(0)
	int16_0 := int16(0)
	int32_0 := int32(0)
	int64_0 := int64(0)
	RegTypes(symPtr, &int_0, &int8_0, &int16_0, &int32_0, &int64_0)

	uint_0 := uint(0)
	uint8_0 := uint8(0)
	uint16_0 := uint16(0)
	uint32_0 := uint32(0)
	uint64_0 := uint64(0)
	RegTypes(symPtr, &uint_0, &uint8_0, &uint16_0, &uint32_0, &uint64_0)

	float32_0 := float32(0)
	float64_0 := float64(0)
	complex64_0 := complex64(0)
	complex128_0 := complex128(0)
	RegTypes(symPtr, &float32_0, &float64_0, &complex64_0, &complex128_0)

	bool_true := true
	string_empty := ""
	unsafe_pointer := unsafe.Pointer(&int_0)
	uintptr_ := uintptr(0)
	RegTypes(symPtr, &bool_true, &string_empty, unsafe_pointer, uintptr_)

	RegTypes(symPtr, []int{}, []int8{}, []int16{}, []int32{}, []int64{})
	RegTypes(symPtr, []uint{}, []uint8{}, []uint16{}, []uint32{}, []uint64{})
	RegTypes(symPtr, []float32{}, []float64{}, []complex64{}, []complex128{})
	RegTypes(symPtr, []bool{}, []string{}, []unsafe.Pointer{}, []uintptr{})
}

func RegSymbol(symPtr map[string]uintptr) {
	regBasicSymbol(symPtr)
	ex, err := os.Executable()
	assert(err)
	f, err := objfile.Open(ex)
	assert(err)
	defer f.Close()

	syms, err := f.Symbols()
	codeType := 'T'
	for _, sym := range syms {
		if sym.Name == "runtime.init" && sym.Code == 't' {
			codeType = 't'
			break
		}
	}
	for _, sym := range syms {
		if sym.Code == codeType && !strings.HasPrefix(sym.Name, "type..") {
			symPtr[sym.Name] = uintptr(sym.Addr)
		} else if strings.HasPrefix(sym.Name, "runtime.") {
			symPtr[sym.Name] = uintptr(sym.Addr)
		}
		if strings.HasPrefix(sym.Name, "go.itab") {
			RegItab(symPtr, sym.Name, uintptr(sym.Addr))
		}
	}
}

func RegItab(symPtr map[string]uintptr, name string, addr uintptr) {
	symPtr[name] = uintptr(addr)
	bs := strings.TrimLeft(name, "go.itab.")
	bss := strings.Split(bs, ",")
	var slice = sliceHeader{addr, len(bss), len(bss)}
	ptrs := *(*[]uintptr)(unsafe.Pointer(&slice))
	for i, ptr := range ptrs {
		typeName := bss[len(bss)-i-1]
		if typeName[0] == '*' {
			var obj interface{} = reflect.TypeOf(0)
			(*interfaceHeader)(unsafe.Pointer(&obj)).word = unsafe.Pointer(ptr)
			typ := obj.(reflect.Type).Elem()
			obj = typ
			typePtr := uintptr((*interfaceHeader)(unsafe.Pointer(&obj)).word)
			symPtr["type."+typeName[1:]] = typePtr
		}
		symPtr["type."+typeName] = ptr
	}
}

func RegTLS(symPtr map[string]uintptr, offset int) {
	var ptr interface{} = RegSymbol
	var slice = sliceHeader{*(*uintptr)((*interfaceHeader)(unsafe.Pointer(&ptr)).word), offset + 4, offset + 4}
	var bytes = *(*[]byte)(unsafe.Pointer(&slice))
	var tlsValue = uintptr(binary.LittleEndian.Uint32(bytes[offset:]))
	symPtr[TLSNAME] = tlsValue
}

func RegType(symPtr map[string]uintptr, name string, typ interface{}) {
	aHeader := (*interfaceHeader)(unsafe.Pointer(&typ))
	symPtr[name] = aHeader.typ
}

func RegFunc(symPtr map[string]uintptr, name string, f interface{}) {
	var ptr = getFuncPtr(f)
	symPtr[name] = ptr
}

func getFuncPtr(f interface{}) uintptr {
	return *(*uintptr)((*interfaceHeader)(unsafe.Pointer(&f)).word)
}
