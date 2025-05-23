package obj

import (
	"unsafe"
)

const (
	InvalidOffset   = int(-1)
	InvalidIndex    = uint32(0xFFFFFFFF)
	InlinedCallSize = int(unsafe.Sizeof(InlinedCall{}))
)

const (
	EmptyString              = ``
	UNRESOLVED_SYMREF_PREFIX = "unresolvedSymRef#"
	UNRESOLVED_SYMREF_FMT    = UNRESOLVED_SYMREF_PREFIX + "%d#%d"
	ABI0_SUFFIX              = ".abi0"
)
