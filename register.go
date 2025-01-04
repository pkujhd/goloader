package goloader

import (
	"cmd/objfile/objfile"
	"fmt"
	"os"
	"reflect"
	"strings"
	"unsafe"

	"github.com/pkujhd/goloader/constants"
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
			if name != EmptyString {
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

	name := constants.TypePrefix + resolveTypeName(t)
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
	typelinksRegister(symPtr)
	return regSymbol(symPtr, path)
}

func RegSymbolWithPath(symPtr map[string]uintptr, path string) error {
	//register types and functions in exe file, the address of symbol not used for relocateaaa, just
	//for builder check reachable
	err := registerTypesInExe(symPtr, path)
	if err != nil {
		return err
	}
	return regSymbol(symPtr, path)
}

func RegSymbol(symPtr map[string]uintptr) error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	typelinksRegister(symPtr)
	return regSymbol(symPtr, path)
}

func regSymbol(symPtr map[string]uintptr, path string) error {
	f, err := objfile.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

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
		if code == "R" && !strings.HasPrefix(sym.Name, DefaultPkgPath) {
			symPtr[sym.Name] = uintptr(int64(sym.Addr) + addroff)
		}
	}
	return nil
}
