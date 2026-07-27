//go:build (386 || amd64) && go1.23 && !go1.24
// +build 386 amd64
// +build go1.23
// +build !go1.24

package obj

import (
	"cmd/objfile/objfile"
	"os"
)

func _Dummy(dummy bool) {
	if dummy {
		path, _ := os.Executable()
		f, _ := objfile.Open(path)
		f.Entries()[0].Disasm()
	}
}
