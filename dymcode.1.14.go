// +build go1.14
// +build !go1.15

package goloader

import (
	"encoding/binary"
	"strconv"
	"unsafe"
)

const (
	R_PCREL = 16
	// R_TLS_LE, used on 386, amd64, and ARM, resolves to the offset of the
	// thread-local symbol from the thread local base and is used to implement the
	// "local exec" model for tls access (r.Sym is not set on intel platforms but is
	// set to a TLS symbol -- runtime.tlsg -- in the linker when externally linking).
	R_TLS_LE = 17
	// R_METHODOFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	// It is a variant of R_ADDROFF used when linking from the uncommonType of a
	// *rtype, and may be set to zero by the linker if it determines the method
	// text is unreachable by the linked program.
	R_METHODOFF = 25
)

func AddStackObject(code *CodeReloc, fi *funcInfoData, seg *segment, symPtr map[string]uintptr) {
	if len(fi.funcdata) > _FUNCDATA_StackObjects && fi.funcdata[_FUNCDATA_StackObjects] != 0xFFFFFFFF {
		stackObjectRecordSize := unsafe.Sizeof(stackObjectRecord{})
		b := code.Mod.stkmaps[fi.funcdata[_FUNCDATA_StackObjects]]
		n := *(*int)(unsafe.Pointer(&b[0]))
		p := uintptr(unsafe.Pointer(&b[PtrSize]))
		for i := 0; i < n; i++ {
			obj := *(*stackObjectRecord)(unsafe.Pointer(p))
			ptr := uintptr(0)
			ok := false
			for _, v := range fi.Var {
				if v.Offset == (int64)(obj.off) {
					ptr, ok = symPtr[v.Type.Name]
					if !ok {
						ptr, ok = seg.typeSymPtr[v.Type.Name]
					}
					if ok {
						off := PtrSize + i*(int)(stackObjectRecordSize) + PtrSize
						if PtrSize == 4 {
							binary.LittleEndian.PutUint32(b[off:], *(*uint32)(unsafe.Pointer(&ptr)))
						} else {
							binary.LittleEndian.PutUint64(b[off:], *(*uint64)(unsafe.Pointer(&ptr)))
						}
					} else {
						strWrite(&seg.err, "unresolve external:", v.Type.Name, "\n")
					}
					break
				}
			}
			if ok == false {
				r := fi.stkobjReloc[i]
				ptr, ok = symPtr[r.Sym.Name]
				if !ok {
					ptr, ok = seg.typeSymPtr[r.Sym.Name]
				}
				if ok {
					off := PtrSize + i*(int)(stackObjectRecordSize) + PtrSize
					if PtrSize == 4 {
						binary.LittleEndian.PutUint32(b[off:], *(*uint32)(unsafe.Pointer(&ptr)))
					} else {
						binary.LittleEndian.PutUint64(b[off:], *(*uint64)(unsafe.Pointer(&ptr)))
					}
				} else {
					strWrite(&seg.err, "unresolve external:", r.Sym.Name, "\n")
				}
			}
			if ok == false {
				strWrite(&seg.err, "unresolve external:", strconv.Itoa(i), " ", fi.name, "\n")
			}
			p = p + stackObjectRecordSize
		}
	}
}

func AddDeferReturn(code *CodeReloc, fi *funcInfoData) {
	if len(fi.funcdata) > _FUNCDATA_OpenCodedDeferInfo && fi.funcdata[_FUNCDATA_OpenCodedDeferInfo] != 0xFFFFFFFF {
		sym := code.Syms[code.SymMap[fi.name]]
		for _, r := range sym.Reloc {
			if r.SymOff == code.SymMap["runtime.deferreturn"] {
				fi.deferreturn = uint32(r.Offset) - uint32(sym.Offset) - 1
				break
			}
		}
	}
}
