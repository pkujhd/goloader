// +build go1.12 go1.13

package goloader

import (
	"encoding/binary"
	"unsafe"
)

func AddStackObject(code *CodeReloc, fi *funcInfoData, seg *segment, symPtr map[string]uintptr) {
	if len(fi.funcdata) > _FUNCDATA_StackObjects {
		b := code.Mod.stkmaps[fi.funcdata[_FUNCDATA_StackObjects]]
		n := *(*int)(unsafe.Pointer(&b[0]))
		p := uintptr(unsafe.Pointer(&b[PtrSize]))
		for i := 0; i < n; i++ {
			obj := *(*stackObjectRecord)(unsafe.Pointer(p))
			for _, v := range fi.Var {
				if v.Offset == (int64)(obj.off) {
					typeName := v.Type.Name
					ptr, ok := symPtr[typeName]
					if !ok {
						ptr, ok = seg.typeSymPtr[typeName]
					}
					if ok {
						off := PtrSize + i*(int)(unsafe.Sizeof(stackObjectRecord{})) + PtrSize
						if PtrSize == 4 {
							binary.LittleEndian.PutUint32(b[off:], *(*uint32)(unsafe.Pointer(&ptr)))
						} else {
							binary.LittleEndian.PutUint64(b[off:], *(*uint64)(unsafe.Pointer(&ptr)))
						}
					} else {
						strWrite(&seg.err, "unresolve external:", typeName, "\n")
					}
					break
				}
			}
			p = p + unsafe.Sizeof(stackObjectRecord{})
		}
	}
}
