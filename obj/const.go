package obj

import (
	"unsafe"
)

const (
	InvalidOffset     = int(-1)
	InvalidIndex      = uint32(0xFFFFFFFF)
	InlinedCallSize   = int(unsafe.Sizeof(InlinedCall{}))
	EmptyString       = ""
	ABI0Suffix        = ".abi0"
	ABIInternalSuffix = ".abiinternal"
)
