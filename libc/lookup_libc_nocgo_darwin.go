//go:build !cgo && darwin
// +build !cgo,darwin

package libc

import (
	"fmt"
	"github.com/eh-steve/goloader"
	"reflect"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	RTLD_NOW    int32 = 0x00002
	RTLD_GLOBAL int32 = 0x00100
)

var selfDlHandle uintptr

func libc_dlopen_trampoline()

//go:cgo_import_dynamic libc_dlopen dlopen "/usr/lib/libSystem.B.dylib"

func libc_dlsym_trampoline()

//go:cgo_import_dynamic libc_dlsym dlsym "/usr/lib/libSystem.B.dylib"

//go:linkname libcCall runtime.libcCall
func libcCall(fn, arg unsafe.Pointer) int32

var dlopenABI0 uintptr
var dlsymABI0 uintptr

func init() {

	dlopenABIInternal := reflect.ValueOf(libc_dlopen_trampoline).Pointer()
	dlsymABIInternal := reflect.ValueOf(libc_dlsym_trampoline).Pointer()

	// reflect.(*Value).Pointer() will always give the ABIInternal FuncPC, not the ABI0.
	// Since we don't have access to internal/abi.FuncPCABI0, we need to find the ABI0
	// version of the function based on whichever one isn't the ABIInternal PC

	abi0PCs := goloader.FuncPCsABI0([]uintptr{dlopenABIInternal, dlsymABIInternal})
	dlopenABI0 = abi0PCs[0]
	dlsymABI0 = abi0PCs[1]

	if dlopenABI0 == 0 {
		panic("could not find ABI0 version of libc_dlopen_trampoline")
	}
	if dlsymABI0 == 0 {
		panic("could not find ABI0 version of libc_dlsym_trampoline")
	}

	h, errNo := selfOpen()
	if h == 0 {
		panic("failed to open self with dlopen, errNo: " + syscall.Errno(errNo).Error())
	}
	selfDlHandle = h
}

func selfOpen() (uintptr, int32) {
	args := struct {
		soPath    *byte
		flags     int32
		retHandle uintptr
		retErrNo  int32
	}{
		soPath: nil,
		flags:  RTLD_GLOBAL | RTLD_NOW,
	}
	libcCall(unsafe.Pointer(dlopenABI0), unsafe.Pointer(&args))
	runtime.KeepAlive(args)
	return args.retHandle + 2, args.retErrNo // Why +2??
}

func lookup(symName string) uintptr {
	symNameBytes := make([]byte, len(symName)+1)
	copy(symNameBytes, symName)
	symNamePtr := &symNameBytes[0]
	args := struct {
		handle  uintptr
		symName *byte
		retAddr uintptr
	}{
		handle:  selfDlHandle,
		symName: symNamePtr,
	}
	_ = libcCall(unsafe.Pointer(dlsymABI0), unsafe.Pointer(&args))
	runtime.KeepAlive(args)
	runtime.KeepAlive(symNameBytes)
	return args.retAddr
}

func LookupDynamicSymbol(symName string) (uintptr, error) {
	addr := lookup(symName)
	if addr == 0 {
		return 0, fmt.Errorf("failed to lookup symbol '%s'", symName)
	}
	return addr, nil
}
