//go:build darwin && arm64 && cgo
// +build darwin,arm64,cgo

package darwin_arm64

import "github.com/eh-steve/goloader/mmap/darwin_arm64/darwin_arm64_cgo"

func MakeThreadJITCodeExecutable(ptr uintptr, len int) {
	darwin_arm64_cgo.MakeThreadJITCodeExecutable(ptr, len)
}

func WriteProtectDisable() {
	darwin_arm64_cgo.WriteProtectDisable()
}
