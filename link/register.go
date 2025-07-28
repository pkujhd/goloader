package link

import (
	"cmd/objfile/objfile"
	"fmt"
	"github.com/pkujhd/goloader/obj"
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
		registerType(t, symPtr)
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
	return regSymbol(symPtr, path, false)
}

func RegSymbol(symPtr map[string]uintptr) error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	typelinksRegister(symPtr)
	return regSymbol(symPtr, path, false)
}

func regSymbol(symPtr map[string]uintptr, path string, isValidateItab bool) error {
	f, err := objfile.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	syms, err := f.Symbols()
	if err != nil {
		return err
	}
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
		if code == "B" || code == "D" || code == "T" || code == "R" {
			if isItabName(sym.Name) && isValidateItab {
				if validateInterface(symPtr, sym.Name) {
					symPtr[sym.Name] = uintptr(int64(sym.Addr) + addroff)
				}
			} else if !strings.HasPrefix(sym.Name, DefaultPkgPath) && !isTypeName(sym.Name) {
				symPtr[sym.Name] = uintptr(int64(sym.Addr) + addroff)
				if strings.HasSuffix(sym.Name, constants.FunctionWrapperSuffix) {
					nName := strings.TrimSuffix(sym.Name, constants.FunctionWrapperSuffix)
					if _, ok := symPtr[nName]; !ok {
						symPtr[nName] = symPtr[sym.Name]
					}
				}
			}
		}
	}

	// if only abi symbols in runtime environment, set abi internal symbol same as abi0
	for symName, ptr := range symPtr {
		if strings.HasSuffix(symName, obj.ABI0_SUFFIX) {
			nName := strings.TrimSuffix(symName, obj.ABI0_SUFFIX)
			if _, ok := symPtr[nName]; !ok {
				symPtr[nName] = ptr
			}
		}
	}
	return nil
}
