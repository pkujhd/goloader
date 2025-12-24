//go:build go1.21 && !go1.27
// +build go1.21,!go1.27

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
