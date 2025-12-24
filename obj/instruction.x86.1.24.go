//go:build (386 || amd64) && go1.24 && !go1.27
// +build 386 amd64
// +build go1.24
// +build !go1.27

package obj

import (
	"cmd/objfile/disasm"
)

func Dummy() {
	_, _ = disasm.DisasmForFile(nil)
}
