package constants

import "unsafe"

// size
const (
	PtrSize    = 4 << (^uintptr(0) >> 63)
	Uint32Size = int(unsafe.Sizeof(uint32(0)))
	IntSize    = int(unsafe.Sizeof(int(0)))
	UInt64Size = int(unsafe.Sizeof(uint64(0)))
	PageSize   = 1 << 12 //4096
)
