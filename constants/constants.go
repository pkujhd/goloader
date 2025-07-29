package constants

// string match prefix/suffix
const (
	FunctionWrapperSuffix = "-fm"
	StkobjSuffix          = ".stkobj"
	FileSymPrefix         = "gofile.."
	GOTPCRELSuffix        = "·f"
	ABI0_SUFFIX           = ".abi0"
)

const (
	OsStdout           = "os.Stdout"
	DefaultPkgPath     = "main"
	RuntimeDeferReturn = "runtime.deferreturn"
)

const EmptyString = ``
const ZeroByte = byte(0x00)

const (
	InvalidOffset      = int(-1)
	InvalidHandleValue = ^uintptr(0)
)
