package goloader

import (
	"runtime"
	"unsafe"
)

func FuncPCsABI0(abiInternalPCs []uintptr) []uintptr {
	abi0PCs := make([]uintptr, len(abiInternalPCs))
	names := make([]string, len(abiInternalPCs))
	for i := range abiInternalPCs {
		f := runtime.FuncForPC(abiInternalPCs[i])
		if f != nil {
			names[i] = f.Name()
		}
	}

	m := activeModules()[0]

	for _, _f := range m.ftab {
		f := (*_func)(unsafe.Pointer(&(m.pclntable[_f.funcoff])))
		name := getfuncname(f, m)
		entry := m.text + uintptr(f.entryoff)
		if f != nil {
			for i, funcName := range names {
				if name == funcName && entry != abiInternalPCs[i] {
					abi0PCs[i] = entry
				}
			}
		}
	}
	return abi0PCs
}
