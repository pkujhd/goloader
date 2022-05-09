package stackobject

// size
const (
	PtrSize = 4 << (^uintptr(0) >> 63)
)

const (
	EmptyString  = ""
	StkobjSuffix = ".stkobj"
)
