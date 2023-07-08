//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package obj

import (
	_ "unsafe"
)

//go:linkname importPathToPrefix cmd/objfile/goobj.importPathToPrefix
func importPathToPrefix(s string) string

func PathToPrefix(s string) string {
	return importPathToPrefix(s)
}
