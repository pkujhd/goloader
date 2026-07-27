//go:build go1.21 && !go1.23
// +build go1.21,!go1.23

package link

import (
	"unsafe"
)

//go:linkname doInit1 runtime.doInit1
func doInit1(t unsafe.Pointer) // t should be a *runtime.initTask
