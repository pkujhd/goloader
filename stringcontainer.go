package goloader

import (
	"unsafe"
)

type stringMmap struct {
	size  int
	index int
	bytes []byte
	addr  uintptr
}

var stringContainer stringMmap = stringMmap{size: DefaultStringContainerSize, index: 0, addr: 0}

func initStringMmap() (err error) {
	stringContainer.bytes, err = Mmap(stringContainer.size)
	if err == nil {
		stringContainer.addr = uintptr(unsafe.Pointer(&stringContainer.bytes[0]))
	}
	return
}

func SetStringContainerSize(size int) bool {
	if stringContainer.addr == 0 {
		stringContainer.size = size
		return true
	}
	return false
}

func OpenStringMap() bool {
	if stringContainer.addr == 0 {
		err := initStringMmap()
		if err != nil {
			return false
		}
	}
	return true
}

func CloseStringMap() error {
	stringContainer.size = DefaultStringContainerSize
	err := Munmap(stringContainer.bytes)
	stringContainer.addr = 0
	return err
}

func IsEnableStringMap() bool {
	return stringContainer.addr != 0
}
