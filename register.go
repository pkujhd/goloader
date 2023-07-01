package goloader

import (
	"cmd/objfile/objfile"
	"fmt"
	"os"
	"reflect"
	"strings"
	"unsafe"
)

// !IMPORTANT: only init firstmodule type, avoid load multiple objs but unload non-sequence errors
func typelinksRegister(symPtr map[string]uintptr) {
	md := firstmoduledata
	for _, tl := range md.typelinks {
		t := (*_type)(adduintptr(md.types, int(tl)))
		if md.typemap != nil {
			t = (*_type)(adduintptr(md.typemap[typeOff(tl)], 0))
		}
		registerType(t, symPtr)
	}
	//register function
	for _, f := range md.ftab {
		if int(f.funcoff) < len(md.pclntable) {
			_func := (*_func)(unsafe.Pointer(&(md.pclntable[f.funcoff])))
			name := getfuncname(_func, &md)
			if !strings.HasPrefix(name, TypeDoubleDotPrefix) && name != EmptyString {
				if _, ok := symPtr[name]; !ok {
					symPtr[name] = getfuncentry(_func, md.text)
				}
			}
		}
	}
}

func registerType(t *_type, symPtr map[string]uintptr) {
	if t.Kind() == reflect.Invalid {
		panic("Unexpected invalid kind during registration!")
	}

	pkgpath := t.PkgPath()
	name := t.nameOff(t.str).name()
	name = strings.Replace(name, pkgname(pkgpath), pkgpath, 1)

	if t.tflag&tflagExtraStar != 0 {
		name = name[1:]
	}
	name = TypePrefix + name
	if _, ok := symPtr[name]; ok {
		return
	}
	symPtr[name] = uintptr(unsafe.Pointer(t))

	switch t.Kind() {
	case reflect.Ptr, reflect.Chan, reflect.Array, reflect.Slice:
		registerType(rtypeOf(t.Elem()), symPtr)
	case reflect.Func:
		for i := 0; i < t.NumIn(); i++ {
			registerType(rtypeOf(t.In(i)), symPtr)
		}
		for i := 0; i < t.NumOut(); i++ {
			registerType(rtypeOf(t.Out(i)), symPtr)
		}
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			registerType(rtypeOf(t.Field(i).Type), symPtr)
		}
	case reflect.Map:
		registerType(rtypeOf(t.Key()), symPtr)
		registerType(rtypeOf(t.Elem()), symPtr)
	case reflect.Bool,
		reflect.Int, reflect.Uint,
		reflect.Int64, reflect.Uint64,
		reflect.Int32, reflect.Uint32,
		reflect.Int16, reflect.Uint16,
		reflect.Int8, reflect.Uint8,
		reflect.Float64, reflect.Float32,
		reflect.Complex64, reflect.Complex128,
		reflect.String, reflect.UnsafePointer,
		reflect.Uintptr,
		reflect.Interface:
		// Nothing to do
	default:
		panic(fmt.Sprintf("typelinksregister found unexpected type (kind %s): ", t.Kind()))
	}
}

func RegSymbolWithSo(symPtr map[string]uintptr, path string) error {
	return regSymbol(symPtr, path)
}

func RegSymbol(symPtr map[string]uintptr) error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	return regSymbol(symPtr, path)
}

func regSymbol(symPtr map[string]uintptr, path string) error {
	f, err := objfile.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	typelinksRegister(symPtr)
	syms, err := f.Symbols()
	for _, sym := range syms {
		if sym.Name == OsStdout {
			symPtr[sym.Name] = uintptr(sym.Addr)
		}
	}
	//Address space layout randomization(ASLR)
	//golang 1.15 symbol address has offset, before 1.15 offset is 0
	addroff := int64(uintptr(unsafe.Pointer(&os.Stdout))) - int64(symPtr[OsStdout])
	for _, sym := range syms {
		code := strings.ToUpper(string(sym.Code))
		if code == "B" || code == "D" {
			symPtr[sym.Name] = uintptr(int64(sym.Addr) + addroff)
		}
		if strings.HasPrefix(sym.Name, ItabPrefix) {
			symPtr[sym.Name] = uintptr(int64(sym.Addr) + addroff)
		}
	}
	return nil
}

func getFunctionPtr(function interface{}) uintptr {
	return *(*uintptr)((*emptyInterface)(unsafe.Pointer(&function)).word)
}
