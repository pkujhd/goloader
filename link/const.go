package link

import (
	"unsafe"
)

// size
const (
	_FuncSize          = int(unsafe.Offsetof(_func{}.Nfuncdata)) + int(unsafe.Sizeof(_func{}.Nfuncdata))
	FindFuncBucketSize = int(unsafe.Sizeof(findfuncbucket{}))
	PCHeaderSize       = int(unsafe.Sizeof(pcHeader{}))
	_typeSize          = int(unsafe.Sizeof(_type{}))
	funcTypeSize       = int(unsafe.Sizeof(funcType{}))
	uncommonTypeSize   = int(unsafe.Sizeof(uncommonType{}))
	InvalidHandleValue = ^uintptr(0)
)

const (
	DefaultPkgPath     = "main"
	RuntimeDeferReturn = "runtime.deferreturn"
)
