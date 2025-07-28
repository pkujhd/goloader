package link

import (
	"unsafe"
)

// size
const (
	PtrSize            = 4 << (^uintptr(0) >> 63)
	Uint32Size         = int(unsafe.Sizeof(uint32(0)))
	IntSize            = int(unsafe.Sizeof(int(0)))
	UInt64Size         = int(unsafe.Sizeof(uint64(0)))
	_FuncSize          = int(unsafe.Offsetof(_func{}.Nfuncdata)) + int(unsafe.Sizeof(_func{}.Nfuncdata))
	FindFuncBucketSize = int(unsafe.Sizeof(findfuncbucket{}))
	PCHeaderSize       = int(unsafe.Sizeof(pcHeader{}))
	_typeSize          = int(unsafe.Sizeof(_type{}))
	funcTypeSize       = int(unsafe.Sizeof(funcType{}))
	uncommonTypeSize   = int(unsafe.Sizeof(uncommonType{}))
	InvalidHandleValue = ^uintptr(0)
	InvalidOffset      = int(-1)
	PageSize           = 1 << 12 //4096
)

const (
	EmptyString    = ""
	DefaultPkgPath = "main"
	ZeroByte       = byte(0x00)
)

// runtime symbol
const (
	RuntimeDeferReturn = "runtime.deferreturn"
)

// string match prefix/suffix
const (
	FileSymPrefix  = "gofile.."
	MainPkgPrefix  = "main."
	OsStdout       = "os.Stdout"
	GOTPCRELSuffix = "·f"
)
