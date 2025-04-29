//go:build go1.12 && !go1.25
// +build go1.12,!go1.25

package stackobject

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/dataindex"
)

// See reflect/value.go sliceHeader
type sliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}

//go:linkname add runtime.add
func add(p unsafe.Pointer, x uintptr) unsafe.Pointer

//go:linkname adduintptr runtime.add
func adduintptr(p uintptr, x int) unsafe.Pointer

func addr2stackObjectRecords(addr unsafe.Pointer) *[]stackObjectRecord {
	n := int(*(*uintptr)(addr))
	slice := sliceHeader{
		Data: uintptr(add(addr, uintptr(PtrSize))),
		Len:  n,
		Cap:  n,
	}
	return (*[]stackObjectRecord)(unsafe.Pointer(&slice))
}

func AddStackObject(funcname string, symMap map[string]*obj.Sym, symbolMap map[string]uintptr, noptrdata uintptr) (err error) {
	Func := symMap[funcname].Func
	if Func != nil && len(Func.FuncData) > dataindex.FUNCDATA_StackObjects &&
		Func.FuncData[dataindex.FUNCDATA_StackObjects] != 0 {
		objects := addr2stackObjectRecords(adduintptr(Func.FuncData[dataindex.FUNCDATA_StackObjects], int(noptrdata)))
		stkobjName := strings.TrimSuffix(funcname, obj.ABI0_SUFFIX) + StkobjSuffix
		for i := range *objects {
			name := EmptyString
			if symbol := symMap[stkobjName]; symbol != nil {
				name = symbol.Reloc[i].SymName
			}
			if ptr, ok := symbolMap[name]; ok {
				setStackObjectPtr(&((*objects)[i]), adduintptr(ptr, 0), noptrdata)
			} else {
				return fmt.Errorf("unresolved external Var! Function name:%s index:%d, name:%s", funcname, i, name)
			}
		}
	}
	return nil
}
