package goloader

import "unsafe"

// size
const (
	PtrSize            = 4 << (^uintptr(0) >> 63)
	Uint32Size         = int(unsafe.Sizeof(uint32(0)))
	IntSize            = int(unsafe.Sizeof(int(0)))
	UInt64Size         = int(unsafe.Sizeof(uint64(0)))
	_FuncSize          = int(unsafe.Sizeof(_func{}))
	ItabSize           = int(unsafe.Sizeof(itab{}))
	FindFuncBucketSize = int(unsafe.Sizeof(findfuncbucket{}))
	InlinedCallSize    = int(unsafe.Sizeof(inlinedCall{}))
	InvalidHandleValue = ^uintptr(0)
	InvalidOffset      = int(-1)
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
	RUNTIME_DEFERRETURN = "runtime.deferreturn"
)

// string match prefix/suffix
const (
	FILE_SYM_PREFIX        = "gofile.."
	TYPE_IMPORTPATH_PREFIX = "type..importpath."
	TYPE_DOUBLE_DOT_PREFIX = "type.."
	TYPE_PREFIX            = "type."
	ITAB_PREFIX            = "go.itab."
	RUNTIME_PREFIX         = "runtime."
	STKOBJ_SUFFIX          = ".stkobj"
	INLINETREE_SUFFIX      = ".inlinetree"
	OS_STDOUT              = "os.Stdout"
)
