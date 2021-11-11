package goloader

import (
	"unsafe"
)

// size
const (
	PtrSize            = 4 << (^uintptr(0) >> 63)
	Uint32Size         = int(unsafe.Sizeof(uint32(0)))
	IntSize            = int(unsafe.Sizeof(int(0)))
	UInt64Size         = int(unsafe.Sizeof(uint64(0)))
	_FuncSize          = int(unsafe.Offsetof(_func{}.nfuncdata)) + int(unsafe.Sizeof(_func{}.nfuncdata))
	FindFuncBucketSize = int(unsafe.Sizeof(findfuncbucket{}))
	InlinedCallSize    = int(unsafe.Sizeof(inlinedCall{}))
	InvalidHandleValue = ^uintptr(0)
	InvalidOffset      = int(-1)
	InvalidIndex       = uint32(0xFFFFFFFF)
	PageSize           = 1 << 12 //4096
)

const (
	EmptyString    = ""
	DefaultPkgPath = "main"
	EmptyPkgPath   = `""`
	ZeroByte       = byte(0x00)
)

const (
	TLSNAME = "(TLS)"
)

// runtime symbol
const (
	RuntimeDeferReturn = "runtime.deferreturn"
)

// string match prefix/suffix
const (
	FileSymPrefix              = "gofile.."
	MainPkgPrefix              = "main."
	TypeImportPathPrefix       = "type..importpath."
	TypeDoubleDotPrefix        = "type.."
	TypePrefix                 = "type."
	ItabPrefix                 = "go.itab."
	StkobjSuffix               = ".stkobj"
	InlineTreeSuffix           = ".inlinetree"
	OsStdout                   = "os.Stdout"
	TypeStringPerfix           = "go.string."
	DefaultStringContainerSize = 1024 * 1024 * 16
)
