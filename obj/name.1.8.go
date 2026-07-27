//go:build go1.8 && !go1.21
// +build go1.8,!go1.21

package obj

import (
	_ "unsafe"
)

type Name struct {
	Bytes *byte
}

func (n Name) Name() string { return _name(n) }

//go:linkname _name reflect.name.name
func _name(n Name) string
