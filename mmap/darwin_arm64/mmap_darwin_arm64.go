//go:build darwin && arm64
// +build darwin,arm64

package darwin_arm64

/*
#cgo darwin LDFLAGS: -lpthread

#include <pthread.h>
#include <libkern/OSCacheControl.h>

void jit_write_protect(int enable) {
	pthread_jit_write_protect_np(enable);
}
void cache_invalidate(void* start, size_t len) {
	sys_icache_invalidate(start, len);
}
*/
import "C"
import "unsafe"

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
	C.jit_write_protect(C.int(1))
	C.cache_invalidate(unsafe.Pointer(ptr), C.size_t(len))
}

func WriteProtect() {
	C.jit_write_protect(C.int(0))
}
