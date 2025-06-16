//go:build go1.21 && !go1.26
// +build go1.21,!go1.26

package obj

import (
	_ "unsafe"
)

type Name struct {
	bytes *byte
}

func (n Name) Name() string { return _name(n) }

//go:linkname _name internal/abi.Name.Name
func _name(n Name) string
