package jsonunload

func Unload(dataStart, dataEnd uintptr) {
	uncacheTypes(dataStart, dataEnd)
}
