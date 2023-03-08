//go:build openbsd
// +build openbsd

package mmap

import (
	"syscall"
	"unsafe"
)

//go:linkname syscall6X syscall.syscall6X
//go:nosplit
func syscall6X(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)

//go:linkname syscall4 syscall.syscall
//go:nosplit
func syscall4(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno)

func mmap(addr uintptr, length uintptr, prot int, flag int, fd int, pos int64) (ret uintptr, err error) {
	r0, _, e1 := syscall6X(getFunctionPtr(libc_mmap_trampoline), uintptr(addr), uintptr(length), uintptr(prot), uintptr(flag), uintptr(fd), uintptr(pos))
	ret = uintptr(r0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

func libc_mmap_trampoline()

//go:cgo_import_dynamic libc_mmap mmap "libc.so"

func munmap(addr uintptr, length uintptr) (err error) {
	_, _, e1 := syscall4(getFunctionPtr(libc_munmap_trampoline), uintptr(addr), uintptr(length), 0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

func libc_munmap_trampoline()

//go:cgo_import_dynamic libc_munmap munmap "libc.so"

type emptyInterface struct {
	_type unsafe.Pointer
	data  unsafe.Pointer
}

func getFunctionPtr(function interface{}) uintptr {
	return *(*uintptr)((*emptyInterface)(unsafe.Pointer(&function)).data)
}
