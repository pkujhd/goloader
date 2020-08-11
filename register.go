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

// See reflect/value.go sliceHeader
type sliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}

func typelinksinit(symPtr map[string]uintptr) {
	md := firstmoduledata
	for _, tl := range md.typelinks {
		t := (*_type)(adduintptr(md.types, int(tl)))
		if md.typemap != nil {
			t = (*_type)(adduintptr(md.typemap[typeOff(tl)], 0))
		}
		switch t.Kind() {
		case reflect.Ptr:
			name := t.nameOff(t.str).name()
			element := *(**_type)(add(unsafe.Pointer(t), unsafe.Sizeof(_type{})))
			pkgpath := t.PkgPath()
			if element != nil && pkgpath == EMPTY_STRING {
				pkgpath = element.PkgPath()
			}
			name = strings.Replace(name, pkgname(pkgpath), pkgpath, 1)
			if element != nil {
				symPtr[TYPE_PREFIX+name[1:]] = uintptr(unsafe.Pointer(element))
			}
			symPtr[TYPE_PREFIX+name] = uintptr(unsafe.Pointer(t))
		default:
		}
	}
	for _, f := range md.ftab {
		_func := (*_func)(unsafe.Pointer((&md.pclntable[f.funcoff])))
		name := gostringnocopy(&md.pclntable[_func.nameoff])
		if !strings.HasPrefix(name, TYPE_DOUBLE_DOT_PREFIX) && _func.entry < md.etext {
			symPtr[name] = _func.entry
		}
	}
}

func RegSymbol(symPtr map[string]uintptr) error {
	typelinksinit(symPtr)
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
	for _, sym := range syms {
		code := strings.ToUpper(string(sym.Code))
		if code == "B" || code == "D" {
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
