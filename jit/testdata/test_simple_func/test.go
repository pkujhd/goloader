package test_simple_func

import (
	"fmt"
)

func Add(a, b int) int {
	return a + b
}

func HandleBytes(input interface{}) (byteVal []byte, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("%v", v)
		}
	}()
	byteVal = input.([]byte)
	return
}

func TestHeapStrings() string {
	return "string literal"
}
