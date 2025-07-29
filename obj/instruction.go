//go:build !(386 || amd64)
// +build !386,!amd64

package obj

func MarkReloc(text []byte, relocs []Reloc, offset int, archName string) {
}

func GetOpName(op uint) string {
	return constants.EmptyString
}

func IsExtraRegister(regName string) bool {
	return false
}
