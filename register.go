package goloader

import (
	"cmd/objfile/objfile"
	"os"
	"reflect"
	"strings"
	"unsafe"
)

//!IMPORTANT: only init firstmodule type, avoid load multiple objs but unload non-sequence errors
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
			if element != nil && pkgpath == EmptyString {
				pkgpath = element.PkgPath()
			}
			name = strings.Replace(name, pkgname(pkgpath), pkgpath, 1)
			if element != nil {
				symPtr[TypePrefix+name[1:]] = uintptr(unsafe.Pointer(element))
			}
			symPtr[TypePrefix+name] = uintptr(unsafe.Pointer(t))
		default:
			//NOTHING TODO
		}
	}
	registerFunc(&md, symPtr)
}

func RegSymbol(symPtr map[string]uintptr) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	f, err := objfile.Open(exe)
	if err != nil {
		return err
	}
	defer f.Close()

	typelinksinit(symPtr)
	syms, err := f.Symbols()
	for _, sym := range syms {
		if sym.Name == OsStdout {
			symPtr[sym.Name] = uintptr(sym.Addr)
		}
	}
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

func regTLS(symPtr map[string]uintptr, offset int) {
	//FUNCTION HEADER
	//x86/amd64
	//asm:		MOVQ (TLS), CX
	//bytes:	0x488b0c2500000000
	funcptr := getFunctionPtr(regTLS)
	tlsptr := *(*uint32)(adduintptr(funcptr, offset))
	symPtr[TLSNAME] = uintptr(tlsptr)
}

func getFunctionPtr(function interface{}) uintptr {
	return *(*uintptr)((*emptyInterface)(unsafe.Pointer(&function)).word)
}
