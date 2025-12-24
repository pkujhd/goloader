//go:build go1.22 && !go1.27
// +build go1.22,!go1.27

package link

import (
	"strings"
	"unsafe"
)

// see $GOROOT/src/internal/abi/switch.go

var typeAssertCacheSlice []uintptr = nil
var typeAssertEmptyCache uintptr = 0

var interfaceSwitchCacheSlice []uintptr = nil
var interfaceEmptySwitchCache uintptr = 0

func registerTypeAssertInterfaceSwitchCache(symPtr map[string]uintptr) {
	typeAssertIndex := 0
	interfaceSwitchIndex := 0
	typeAssertEmptyCache = symPtr["runtime.emptyTypeAssertCache"]
	interfaceEmptySwitchCache = symPtr["runtime.emptyInterfaceSwitchCache"]
	for symName, _ := range symPtr {
		if strings.Contains(symName, "..typeAssert.") {
			typeAssertIndex++
		}
		if strings.Contains(symName, "..interfaceSwitch.") {
			interfaceSwitchIndex++
		}
	}

	typeAssertCacheSlice = make([]uintptr, typeAssertIndex)
	interfaceSwitchCacheSlice = make([]uintptr, interfaceSwitchIndex)
	typeAssertIndex = 0
	interfaceSwitchIndex = 0
	for symName, ptr := range symPtr {
		if strings.Contains(symName, "..typeAssert.") {
			typeAssertCacheSlice[typeAssertIndex] = ptr
			typeAssertIndex++
		}
		if strings.Contains(symName, "..interfaceSwitch.") {
			interfaceSwitchCacheSlice[interfaceSwitchIndex] = ptr
			interfaceSwitchIndex++
		}
	}

}

func resetTypeAssertInterfaceSwitchCache() {
	for _, cachePtr := range typeAssertCacheSlice {
		if cachePtr != uintptr(0) {
			*(*uintptr)(unsafe.Pointer(cachePtr)) = typeAssertEmptyCache
		}
	}
	for _, cachePtr := range interfaceSwitchCacheSlice {
		if cachePtr != uintptr(0) {
			*(*uintptr)(unsafe.Pointer(cachePtr)) = interfaceEmptySwitchCache
		}
	}
}
