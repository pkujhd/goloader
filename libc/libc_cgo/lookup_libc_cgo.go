//go:build cgo && !windows
// +build cgo,!windows

package libc_cgo

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <limits.h>
#include <stdlib.h>
#include <stdint.h>

#include <stdio.h>

static uintptr_t selfOpen(char** err) {
	void* h = dlopen(NULL, RTLD_NOW|RTLD_GLOBAL);
	if (h == NULL) {
		*err = (char*)dlerror();
	}
	return (uintptr_t)h;
}

static void* lookup(uintptr_t h, const char* name, char** err) {
	void* r = dlsym((void*)h, name);
	if (r == NULL) {
		*err = (char*)dlerror();
	}
	return r;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

var selfDlHandle C.uintptr_t

func init() {
	var cErr *C.char
	h := C.selfOpen(&cErr)
	if h == 0 {
		panic(C.GoString(cErr))
	}
	selfDlHandle = h
}

func LookupDynamicSymbol(symName string) (uintptr, error) {
	cName := C.CString(symName)
	defer C.free(unsafe.Pointer(cName))
	var cErr *C.char
	addr := C.lookup(selfDlHandle, cName, &cErr)
	if addr == nil {
		return 0, fmt.Errorf("failed to lookup symbol %s: %s", symName, string(C.GoString(cErr)))
	}
	return uintptr(addr), nil
}
