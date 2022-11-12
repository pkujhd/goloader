package goloader

import (
	"cmd/objfile/objfile"
	"debug/elf"
	"fmt"
	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/dataindex"
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
			numStars := 2
			if element != nil && pkgpath == EmptyString {
				switch element.Kind() {
				case reflect.Slice, reflect.Chan, reflect.Array:
					numStars = 1
					fallthrough
				case reflect.Ptr:
					elementElem = *(**_type)(add(unsafe.Pointer(element), unsafe.Sizeof(_type{})))
				case reflect.Interface:
					numStars = 1
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
					symPtr[TypePrefix+name[numStars:]] = uintptr(unsafe.Pointer(elementElem))
				}
			}
			symPtr[TypePrefix+name] = uintptr(unsafe.Pointer(t))
		case reflect.Chan, reflect.Array, reflect.Slice, reflect.Struct, reflect.Func:
			pkgpath := t.PkgPath()
			name := fullyQualifiedName(t, pkgpath)
			// These type names all start with *, but their elem will generally be a pointer and so picked up in the case aboveabove
			if name[0] == '*' {
				symPtr[TypePrefix+name[1:]] = uintptr(unsafe.Pointer(t))
			} else {
				panic("unexpected " + t.Kind().String() + " type" + name)
			}
		case reflect.Map:
			mt := (*mapType)(adduintptr(md.types, int(tl)))
			for _, element := range []*_type{mt.key, mt.elem} {
				var elementElem *_type
				pkgpath := t.PkgPath()
				if element != nil && pkgpath == EmptyString {
					switch element.Kind() {
					case reflect.Ptr, reflect.Array, reflect.Slice, reflect.Chan:
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
			}
		default:
			panic(fmt.Sprintf("typelinksregister found unexpected type (kind %s): %s", t.Kind(), _name(t.nameOff(t.str))))
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

					// Asm function ABI wrappers will usually be inlined away into the caller's code, but it may be
					// useful to know that certain functions are ABI0 and so cannot be called from Go directly
					if _func.flag&funcFlag_ASM > 0 && _func.args == dataindex.ArgsSizeUnknown {
						// Make clear that the ASM func uses ABI0 not ABIInternal by storing another suffixed copy

						symPtr[name+obj.ABI0Suffix] = symPtr[name]
					}
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

var resolvedTlsG uintptr = 0

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

	tlsG, x86Found := symPtr["runtime.tlsg"]
	tls_G, arm64Found := symPtr["runtime.tls_g"]

	if resolvedTlsG != 0 {
		symPtr[TLSNAME] = resolvedTlsG
	} else {
		if x86Found || arm64Found {
			// If this is an ELF file, try to relocate the tls G as created by the external linker
			path, err := os.Executable()
			if err != nil {
				return fmt.Errorf("found 'runtime.tlsg' and so expected elf file (macho not yet supported), but failed to find executable: %w", err)
			}
			elfFile, err := elf.Open(path)
			if err != nil {
				return fmt.Errorf("found 'runtime.tlsg' and so expected elf file (macho not yet supported), but failed to open ELF executable: %w", err)
			}
			defer elfFile.Close()

			var tls *elf.Prog
			for _, prog := range elfFile.Progs {
				if prog.Type == elf.PT_TLS {
					tls = prog
					break
				}
			}
			if tls == nil {
				tlsG = uintptr(^uint64(PtrSize) + 1) // -ptrSize
			} else {
				// Copied from delve/pkg/proc/bininfo.go
				switch elfFile.Machine {
				case elf.EM_X86_64, elf.EM_386:

					// According to https://reviews.llvm.org/D61824, linkers must pad the actual
					// size of the TLS segment to ensure that (tlsoffset%align) == (vaddr%align).
					// This formula, copied from the lld code, matches that.
					// https://github.com/llvm-mirror/lld/blob/9aef969544981d76bea8e4d1961d3a6980980ef9/ELF/InputSection.cpp#L643
					memsz := uintptr(tls.Memsz + (-tls.Vaddr-tls.Memsz)&(tls.Align-1))

					// The TLS register points to the end of the TLS block, which is
					// tls.Memsz long. runtime.tlsg is an offset from the beginning of that block.
					tlsG = ^(memsz) + 1 + tlsG // -tls.Memsz + tlsg.Value

				case elf.EM_AARCH64:
					if !arm64Found || tls == nil {
						tlsG = uintptr(2 * uint64(PtrSize))
					} else {
						tlsG = tls_G + uintptr(PtrSize*2) + ((uintptr(tls.Vaddr) - uintptr(PtrSize*2)) & uintptr(tls.Align-1))
					}

				default:
					// we should never get here
					return fmt.Errorf("found 'runtime.tlsg' but got unsupported architecture: %s", elfFile.Machine)
				}
			}
			resolvedTlsG = resolvedTlsG
			symPtr[TLSNAME] = tlsG
		}
	}

	return nil
}

func getFunctionPtr(function interface{}) uintptr {
	return *(*uintptr)((*emptyInterface)(unsafe.Pointer(&function)).data)
}
