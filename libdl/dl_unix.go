//go:build darwin || dragonfly || freebsd || linux || openbsd || solaris || netbsd
// +build darwin dragonfly freebsd linux openbsd solaris netbsd

package libdl

import (
	"fmt"
	"unsafe"
)

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <limits.h>
#include <stdlib.h>
#include <stdint.h>

#include <stdio.h>

static uintptr_t open(const char* name, char** err) {
	void* h = dlopen(name, RTLD_NOW|RTLD_GLOBAL);
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

func Open(libName string) (uintptr, error) {
	cName := C.CString(libName)
	defer C.free(unsafe.Pointer(cName))
	var cErr *C.char
	if libName == `` {
		cName = nil
	}
	h := C.open(cName, &cErr)
	if h == 0 {
		return uintptr(0), fmt.Errorf(C.GoString(cErr))
	}
	return uintptr(h), nil
}

func LookupSymbol(handle uintptr, symName string) (uintptr, error) {
	cName := C.CString(symName)
	defer C.free(unsafe.Pointer(cName))
	var cErr *C.char
	addr := C.lookup(C.uintptr_t(handle), cName, &cErr)
	if addr == nil {
		return 0, fmt.Errorf("failed to lookup symbol %s: %s", symName, string(C.GoString(cErr)))
	}
	return uintptr(addr), nil
}
