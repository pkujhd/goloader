//go:build go1.9 && !go1.24
// +build go1.9,!go1.24

package obj

import "cmd/objfile/objabi"

func PathToPrefix(s string) string {
	str := objabi.PathToPrefix(s)
	// golang >= 1.18, go.shape is a special builtin package whose name shouldn't be escaped
	if str == "go%2esharp" {
		str = "go.sharp"
	}
	return str
}
