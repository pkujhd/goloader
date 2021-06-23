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

	typelinksinit(symPtr)
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

func regTLS(symPtr map[string]uintptr, oper []byte) {
	//FUNCTION HEADER
	//x86/amd64
	//asm:		MOVQ (TLS), CX
	//bytes:	0x64488b0c2500000000	or	0x65488b0c2500000000
	//FS(0x64) or GS(0x65) segment register, on golang 1.8, FS/GS all generate.
	//asm:		MOVQ OFF(IP), CX
	//bytes:	0x488b0d00000000
	//MOVQ OFF(IP), CX will be generated when goloader is a c-typed dynamic lib(only on linux/amd64)
	funcptr := getFunctionPtr(regTLS)
	for i := 0; i < len(oper); i++ {
		if *(*byte)(adduintptr(funcptr, i)) != oper[i] {
			//function header modified
			if *(*uint32)(unsafe.Pointer(funcptr))&0x00FFFFFF == 0x000d8b48 {
				ptr := *(*uint32)(adduintptr(funcptr, 3))
				ptr = *(*uint32)(adduintptr(funcptr, int(ptr)+7))
				symPtr[TLSNAME] = uintptr(ptr)
				return
			} else {
				panic("function header modified, can not relocate TLS")
			}
		}
	}
	tlsptr := *(*uint32)(adduintptr(funcptr, len(oper)))
	symPtr[TLSNAME] = uintptr(tlsptr)
}

func getFunctionPtr(function interface{}) uintptr {
	return *(*uintptr)((*emptyInterface)(unsafe.Pointer(&function)).word)
}
