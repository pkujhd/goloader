package obj

import (
	"unsafe"
)

const (
	InvalidIndex             = uint32(0xFFFFFFFF)
	InlinedCallSize          = int(unsafe.Sizeof(InlinedCall{}))
	UNRESOLVED_SYMREF_PREFIX = "unresolvedSymRef#"
	UNRESOLVED_SYMREF_FMT    = UNRESOLVED_SYMREF_PREFIX + "%d#%d"
)
