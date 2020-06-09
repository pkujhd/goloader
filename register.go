package goloader

import (
	"cmd/objfile/objfile"
	"os"
	"reflect"
	"strings"
	"unsafe"
)

// See reflect/value.go emptyInterface
type emptyInterface struct {
	typ  unsafe.Pointer
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
	string_empty := EMPTY_STRING
	unsafe_pointer := unsafe.Pointer(&int_0)
	uintptr_ := uintptr(0)
	RegTypes(symPtr, &bool_true, &string_empty, unsafe_pointer, uintptr_)

	RegTypes(symPtr, []int{}, []int8{}, []int16{}, []int32{}, []int64{})
	RegTypes(symPtr, []uint{}, []uint8{}, []uint16{}, []uint32{}, []uint64{})
	RegTypes(symPtr, []float32{}, []float64{}, []complex64{}, []complex128{})
	RegTypes(symPtr, []bool{}, []string{}, []unsafe.Pointer{}, []uintptr{})
}

func RegSymbol(symPtr map[string]uintptr) error {
	regBasicSymbol(symPtr)
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	f, err := objfile.Open(exe)
	if err != nil {
		return err
	}
	defer f.Close()

	syms, err := f.Symbols()
	codeType := 'T'
	for _, sym := range syms {
		if sym.Name == RUNTIME_INIT && sym.Code == 't' {
			codeType = 't'
			break
		}
	}
	for _, sym := range syms {
		if sym.Code == codeType && !strings.HasPrefix(sym.Name, TYPE_DOUBLE_DOT_PREFIX) {
			symPtr[sym.Name] = uintptr(sym.Addr)
		} else if strings.HasPrefix(sym.Name, RUNTIME_PREFIX) {
			symPtr[sym.Name] = uintptr(sym.Addr)
		}
		if strings.HasPrefix(sym.Name, ITAB_PREFIX) {
			regItab(symPtr, sym.Name, uintptr(sym.Addr))
		}
	}
	return nil
}

func regItab(symPtr map[string]uintptr, name string, addr uintptr) {
	symPtr[name] = addr
	bss := strings.Split(strings.TrimLeft(name, ITAB_PREFIX), ",")
	slice := sliceHeader{addr, len(bss), len(bss)}
	ptrs := *(*[]unsafe.Pointer)(unsafe.Pointer(&slice))
	for i, ptr := range ptrs {
		tname := bss[len(bss)-i-1]
		if tname[0] == '*' {
			obj := reflect.TypeOf(0)
			(*emptyInterface)(unsafe.Pointer(&obj)).word = ptr
			obj = obj.(reflect.Type).Elem()
			symPtr[TYPE_PREFIX+tname[1:]] = uintptr((*emptyInterface)(unsafe.Pointer(&obj)).word)
		}
		symPtr[TYPE_PREFIX+tname] = uintptr(ptr)
	}
}

func regTLS(symPtr map[string]uintptr, offset int) {
	//FUNCTION HEADER
	//x86/amd64
	//asm:		MOVQ (TLS), CX
	//bytes:	0x488b0c2500000000
	funcptr := getFunctionPtr(regTLS)
	tlsptr := *(*uint32)(adduintptr(funcptr, offset))
	symPtr[TLSNAME] = uintptr(tlsptr)
}

func regFunc(symPtr map[string]uintptr, name string, function interface{}) {
	symPtr[name] = getFunctionPtr(function)
}

func getFunctionPtr(function interface{}) uintptr {
	return *(*uintptr)((*emptyInterface)(unsafe.Pointer(&function)).word)
}
