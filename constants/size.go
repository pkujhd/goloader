package constants

// size
const (
	PtrSize = 4 << (^uintptr(0) >> 63)
)
