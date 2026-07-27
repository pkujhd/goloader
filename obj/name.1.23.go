//go:build go1.23 && !go1.28
// +build go1.23,!go1.28

package obj

type Name struct {
	Bytes *byte
}

func (n Name) Name() string { return _name(n) }
