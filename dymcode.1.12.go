// +build go1.12 go1.13
// +build !go1.14

package goloader

import (
	"encoding/binary"
	"unsafe"
)

func AddStackObject(code *CodeReloc, fi *funcInfoData, seg *segment, symPtr map[string]uintptr) {
	if len(fi.funcdata) > _FUNCDATA_StackObjects {
		stackObjectRecordSize := unsafe.Sizeof(stackObjectRecord{})
		b := code.Mod.stkmaps[fi.funcdata[_FUNCDATA_StackObjects]]
		n := *(*int)(unsafe.Pointer(&b[0]))
		p := uintptr(unsafe.Pointer(&b[PtrSize]))
		for i := 0; i < n; i++ {
			obj := *(*stackObjectRecord)(unsafe.Pointer(p))
			for _, v := range fi.Var {
				if v.Offset == (int64)(obj.off) {
					ptr, ok := symPtr[v.Type.Name]
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
			p = p + stackObjectRecordSize
		}
	}
}
