package goloader

type stringMmap struct {
	size  int
	index int
	bytes []byte
	addr  uintptr
}
