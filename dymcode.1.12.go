// +build go1.12
// +build !go1.14,!go1.15

package goloader

import (
	"encoding/binary"
	"strconv"
	"unsafe"
)

const (
	R_PCREL = 15
	// R_TLS_LE, used on 386, amd64, and ARM, resolves to the offset of the
	// thread-local symbol from the thread local base and is used to implement the
	// "local exec" model for tls access (r.Sym is not set on intel platforms but is
	// set to a TLS symbol -- runtime.tlsg -- in the linker when externally linking).
	R_TLS_LE = 16
	// R_METHODOFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	// It is a variant of R_ADDROFF used when linking from the uncommonType of a
	// *rtype, and may be set to zero by the linker if it determines the method
	// text is unreachable by the linked program.
	R_METHODOFF = 24
)

func AddStackObject(code *CodeReloc, fi *funcInfoData, seg *segment, symPtr map[string]uintptr) {
	if len(fi.funcdata) > _FUNCDATA_StackObjects && fi.funcdata[_FUNCDATA_StackObjects] != 0xFFFFFFFF {
		stackObjectRecordSize := unsafe.Sizeof(stackObjectRecord{})
		b := code.Mod.stkmaps[fi.funcdata[_FUNCDATA_StackObjects]]
		n := *(*int)(unsafe.Pointer(&b[0]))
		p := unsafe.Pointer(&b[PtrSize])
		for i := 0; i < n; i++ {
			obj := *(*stackObjectRecord)(p)
			var name string
			for _, v := range fi.Var {
				if v.Offset == (int64)(obj.off) {
					name = v.Type.Name
					break
				}
			}
			if len(name) == 0 {
				name = fi.stkobjReloc[i].Sym.Name
			}
			ptr, ok := symPtr[name]
			if !ok {
				ptr, ok = seg.typeSymPtr[name]
			}
			if !ok {
				sprintf(&seg.err, "unresolve external:", strconv.Itoa(i), " ", fi.name, "\n")
			} else {
				off := PtrSize + i*(int)(stackObjectRecordSize) + PtrSize
				if PtrSize == 4 {
					binary.LittleEndian.PutUint32(b[off:], *(*uint32)(unsafe.Pointer(&ptr)))
				} else {
					binary.LittleEndian.PutUint64(b[off:], *(*uint64)(unsafe.Pointer(&ptr)))
				}
			}
			p = add(p, stackObjectRecordSize)
		}
	}
}

func AddDeferReturn(code *CodeReloc, fi *funcInfoData, seg *segment) {
}
