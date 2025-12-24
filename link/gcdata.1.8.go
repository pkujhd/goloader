//go:build go1.8 && !go1.24
// +build go1.8,!go1.24

package link

import (
	"cmd/objfile/gcprog"
	"unsafe"

	"github.com/pkujhd/goloader/constants"
)

const (
	KindGCProg = 1 << 6
)

func gcDataAddType(linker *Linker, w *gcprog.Writer, off int64, typ *_type) {
	ptrData := int64(typ.ptrdata) / int64(linker.Arch.PtrSize)
	if typ.kind&KindGCProg == 0 {
		var mask []byte
		append2Slice(&mask, uintptr(unsafe.Pointer(typ.gcdata)), int(ptrData+7)/8)
		for i := int64(0); i < ptrData; i++ {
			if (mask[i/8]>>uint(i%8))&1 != 0 {
				w.Ptr(off/int64(linker.Arch.PtrSize) + i)
			}
		}
	} else {
		var prog []byte
		append2Slice(&prog, uintptr(unsafe.Pointer(typ.gcdata))+uintptr(constants.Uint32Size), int(*(*uint32)(unsafe.Pointer(typ.gcdata))))
		w.ZeroUntil(off / int64(linker.Arch.PtrSize))
		w.Append(prog, ptrData)
	}
}
