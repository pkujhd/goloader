//go:build go1.20 && !go1.22
// +build go1.20,!go1.22

package obj

//golang 1.20 change magic number
var (
	ModuleHeadx86 = []byte{0xF1, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x1, 0x0}
	ModuleHeadarm = []byte{0xF1, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x4, 0x0}
)
