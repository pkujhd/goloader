//go:build (386 || amd64) && go1.24 && !go1.25
// +build 386 amd64
// +build go1.24
// +build !go1.25

package obj

import (
	"cmd/objfile/disasm"
)

func Dummy() {
	_, _ = disasm.DisasmForFile(nil)
}
