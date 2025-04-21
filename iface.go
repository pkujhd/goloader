package goloader

import (
	"github.com/pkujhd/goloader/obj"
	"strings"
	"unsafe"

	"github.com/pkujhd/goloader/constants"
)

// Mutual exclusion locks.  In the uncontended case,
// as fast as spin locks (just a few user-level instructions),
// but on the contention path they sleep in the kernel.
// A zeroed Mutex is unlocked (no need to initialize each lock).
type mutex struct {
	// Futex-based impl treats it as uint32 key,
	// while sema-based impl as M* waitm.
	// Used to be a union, but unions break precise GC.
	key uintptr
}

//go:linkname lock runtime.lock
func lock(l *mutex)

//go:linkname unlock runtime.unlock
func unlock(l *mutex)

//go:linkname atomicstorep runtime.atomicstorep
func atomicstorep(ptr unsafe.Pointer, new unsafe.Pointer)

//go:linkname getitab runtime.getitab
func getitab(inter *interfacetype, typ *_type, canfail bool) *itab

func validateInterface(symPtr map[string]uintptr, name string) bool {
	result := strings.Split(name, ",")
	interTypeName := constants.TypePrefix + result[1]
	inter := (*interfacetype)(unsafe.Pointer(symPtr[interTypeName]))
	typeName := constants.TypePrefix + strings.TrimPrefix(result[0], constants.ItabPrefix)
	typ := (*_type)(unsafe.Pointer(symPtr[typeName]))

	if inter != nil && typ != nil {
		x := typ.uncommon()
		off := add(unsafe.Pointer(x), uintptr(x.moff))
		ni := len(inter.mhdr)
		methods := (*[1 << 16]unsafe.Pointer)(unsafe.Pointer(off))[:ni:ni]
		for i := 0; i < ni; i++ {
			if uintptr(methods[i]) == InvalidHandleValue {
				return false
			}
		}
		nt := int(x.mcount)
		xmhdr := (*[1 << 16]method)(add(unsafe.Pointer(x), uintptr(x.moff)))[:nt:nt]
		for k := 0; k < nt; k++ {
			t := &xmhdr[k]
			if int(t.ifn) == InvalidOffset || int(t.tfn) == InvalidOffset {
				return false
			}
		}
		return true
	}
	return false
}

func getUnimplementedInterfaceType(symbol *obj.Sym, symPtr map[string]uintptr) []string {
	methods := make(map[string]string)
	for i := len(symbol.Reloc) - 1; i > 0; i -= 2 {
		if strings.Contains(symbol.Reloc[i].SymName, constants.TypeImportPathPrefix) {
			break
		}
		methodName := strings.TrimSuffix(strings.TrimPrefix(symbol.Reloc[i-1].SymName, constants.TypeNameDataPrefix), ".")
		methods[methodName] = symbol.Reloc[i].SymName
	}

	if len(methods) == 0 {
		return nil
	}

	typeNames := make([]string, 0)
	for typeName, p := range symPtr {
		if isTypeName(typeName) {
			typ := (*_type)(unsafe.Pointer(p))
			if isTypeImpleltementMethods(typ, methods) && hasInvalidMethod(typ, methods) {
				typeNames = append(typeNames, typeName)
			}
		}
	}
	return typeNames
}

func isTypeImpleltementMethods(typ *_type, methods map[string]string) bool {
	uncommon := _uncommon(typ)
	if uncommon != nil && int(uncommon.mcount) >= len(methods) {
		for methodName, typeName := range methods {
			methodFound := false
			for _, m := range uncommon.methods() {
				if methodName == typ.nameOff(m.name).Name() {
					tName := EmptyString
					if m.mtyp != constants.InvalidTypeOff && m.mtyp != constants.UnReachableTypeOff {
						_typ := typ.typeOff(m.mtyp)
						tName = constants.TypePrefix + _typ.String()
					}
					if typeName == tName || tName == EmptyString {
						methodFound = true
						break
					}
				}
			}
			if !methodFound {
				return false
			}
		}
		return true
	}
	return false
}

func hasInvalidMethod(typ *_type, methods map[string]string) bool {
	uncommon := _uncommon(typ)
	if uncommon != nil {
		for _, m := range uncommon.methods() {
			if m.ifn == constants.InvalidMethodOff {
				methodName := typ.nameOff(m.name).Name()
				methodTypeName := EmptyString
				if m.mtyp != constants.InvalidTypeOff && m.mtyp != constants.UnReachableTypeOff {
					_typ := typ.typeOff(m.mtyp)
					methodTypeName = constants.TypePrefix + _typ.String()
				}
				if mType, ok := methods[methodName]; ok {
					if methodTypeName == mType || methodTypeName == EmptyString {
						return true
					}
				}
			}
		}
	}
	return false
}
