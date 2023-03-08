//go:build darwin && go1.20
// +build darwin,go1.20

package mmap

import (
	"reflect"
	_ "unsafe"
)

var _ = reflect.ValueOf(syscall_syscall9)

//go:linkname syscall_syscall9 syscall.syscall9
func syscall_syscall9(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9 uintptr) (r1, r2, err uintptr)
