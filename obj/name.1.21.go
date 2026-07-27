//go:build go1.21 && !go1.23
// +build go1.21,!go1.23

package obj

import (
	_ "unsafe"
)

type Name struct {
	Bytes *byte
}

func (n Name) Name() string { return _name(n) }

//go:linkname _name internal/abi.Name.Name
func _name(n Name) string
