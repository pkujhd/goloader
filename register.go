package goloader

import (
	"cmd/objfile/objfile"
	"os"
	"reflect"
	"strings"
	"unsafe"
)

//go:linkname typelinksinit runtime.typelinksinit
func typelinksinit()

// !IMPORTANT: only init firstmodule type, avoid load multiple objs but unload non-sequence errors
func typelinksregister(symPtr map[string]uintptr, pkgSet map[string]struct{}) {
	md := firstmoduledata
	for _, tl := range md.typelinks {
		t := (*_type)(adduintptr(md.types, int(tl)))
		if md.typemap != nil {
			t = md.typemap[typeOff(tl)]
		}

		switch t.Kind() {
		case reflect.Ptr:
			element := *(**_type)(add(unsafe.Pointer(t), unsafe.Sizeof(_type{})))
			var elementElem *_type
			pkgpath := t.PkgPath()
			if element != nil && pkgpath == EmptyString {
				switch element.Kind() {
				case reflect.Ptr, reflect.Array, reflect.Slice:
					elementElem = *(**_type)(add(unsafe.Pointer(element), unsafe.Sizeof(_type{})))
				}
				pkgpath = element.PkgPath()
				if elementElem != nil && pkgpath == EmptyString {
					pkgpath = elementElem.PkgPath()
				}
			}
			pkgSet[pkgpath] = struct{}{}
			name := fullyQualifiedName(t, pkgpath)
			if element != nil {
				symPtr[TypePrefix+name[1:]] = uintptr(unsafe.Pointer(element))
				if elementElem != nil {
					symPtr[TypePrefix+name[2:]] = uintptr(unsafe.Pointer(elementElem))
				}
			}
			symPtr[TypePrefix+name] = uintptr(unsafe.Pointer(t))
		default:
			//NOTHING TODO
		}
	}
	//register function
	for _, f := range md.ftab {
		if int(f.funcoff) < len(md.pclntable) {
			_func := (*_func)(unsafe.Pointer(&(md.pclntable[f.funcoff])))
			name := getfuncname(_func, &md)
			if name != EmptyString {
				if _, ok := symPtr[name]; !ok {
					pkgpath := funcPkgPath(name)
					if name != pkgpath+_InitTaskSuffix {
						// Don't add to the package list if the only thing included is the init func
						pkgSet[pkgpath] = struct{}{}
					}
					symPtr[name] = getfuncentry(_func, md.text)
				}
			}
		}
	}
}

func RegSymbolWithSo(symPtr map[string]uintptr, pkgSet map[string]struct{}, path string) error {
	return regSymbol(symPtr, pkgSet, path)
}

func RegSymbol(symPtr map[string]uintptr, pkgSet map[string]struct{}) error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	return regSymbol(symPtr, pkgSet, path)
}

func regSymbol(symPtr map[string]uintptr, pkgSet map[string]struct{}, path string) error {
	f, err := objfile.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	typelinksregister(symPtr, pkgSet)
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
