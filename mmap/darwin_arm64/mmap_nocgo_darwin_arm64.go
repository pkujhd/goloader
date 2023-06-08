//go:build darwin && arm64 && !cgo
// +build darwin,arm64,!cgo

package darwin_arm64

import (
	"reflect"
	"runtime"
	"unsafe"
)

//go:linkname libcCall runtime.libcCall
func libcCall(fn, arg unsafe.Pointer) int32

func pthread_jit_write_protect_np_trampoline()

func sys_icache_invalidate_trampoline()

//go:cgo_import_dynamic libpthread_pthread_jit_write_protect_np pthread_jit_write_protect_np "/usr/lib/libSystem.B.dylib"

//go:cgo_import_dynamic libkern_sys_icache_invalidate sys_icache_invalidate "/usr/lib/libSystem.B.dylib"

//go:linkname FuncPCsABI0 github.com/eh-steve/goloader.FuncPCsABI0
func FuncPCsABI0(abiInternalPCs []uintptr) []uintptr

var jitWriteProtectABI0 uintptr
var sysICacheInvalidateABI0 uintptr

func init() {
	jitWriteProtectABIInternal := reflect.ValueOf(pthread_jit_write_protect_np_trampoline).Pointer()
	sysICacheInvalidateABIInternal := reflect.ValueOf(sys_icache_invalidate_trampoline).Pointer()

	abi0PCs := FuncPCsABI0([]uintptr{jitWriteProtectABIInternal, sysICacheInvalidateABIInternal})
	jitWriteProtectABI0 = abi0PCs[0]
	sysICacheInvalidateABI0 = abi0PCs[1]

	if jitWriteProtectABI0 == 0 {
		panic("could not find ABI0 PC of pthread_jit_write_protect_np_trampoline")
	}
	if sysICacheInvalidateABI0 == 0 {
		panic("could not find ABI0 PC of sys_icache_invalidate_trampoline")
	}
}

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
	args := struct {
		writeProtect int32
	}{
		writeProtect: 1,
	}
	libcCall(unsafe.Pointer(jitWriteProtectABI0), unsafe.Pointer(&args))
	runtime.KeepAlive(args)

	args2 := struct {
		ptr uintptr
		len int
	}{
		ptr: ptr,
		len: len,
	}
	libcCall(unsafe.Pointer(sysICacheInvalidateABI0), unsafe.Pointer(&args2))
	runtime.KeepAlive(args2)
}

func WriteProtectDisable() {
	args := struct {
		writeProtect int32
	}{
		writeProtect: 0,
	}
	libcCall(unsafe.Pointer(jitWriteProtectABI0), unsafe.Pointer(&args))
	runtime.KeepAlive(args)
}
