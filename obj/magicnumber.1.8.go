//go:build go1.8 && !go1.16
// +build go1.8,!go1.16

package obj

var (
	ModuleHeadx86 = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x1, 0x0}
	ModuleHeadarm = []byte{0xFB, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x4, 0x0}
)