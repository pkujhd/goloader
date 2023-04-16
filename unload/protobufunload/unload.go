package protobufunload

func Unload(dataStart, dataEnd uintptr) {
	deregisterProtobufPackages(dataStart, dataEnd)
}
