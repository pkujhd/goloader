package test_complex_func

import (
	"fmt"
	"github.com/pkujhd/goloader/jit/testdata/common"
)

type Blah struct {
	myMap map[string]interface{}
}

func NewThing() common.SomeInterface {
	return &Blah{
		myMap: map[string]interface{}{},
	}
}

func (b *Blah) Method1(input common.SomeStruct) (common.SomeStruct, error) {
	switch typedVal := input.Val1.(type) {
	case []byte:
		typedVal[0], typedVal[2] = typedVal[2], typedVal[0]
	default:
		return input, fmt.Errorf("expected []byte, got %T", input.Val1)
	}
	input.Val2 = b.myMap
	return input, nil
}

func (b *Blah) Method2(input map[string]interface{}) error {
	for k, v := range input {
		b.myMap[k] = v
	}
	return nil
}

func ComplexFunc(input common.SomeStruct) (common.SomeStruct, error) {
	switch typedVal := input.Val1.(type) {
	case []byte:
		typedVal[0], typedVal[2] = typedVal[2], typedVal[0]
	default:
		return input, fmt.Errorf("expected []byte, got %T", input.Val1)
	}
	return input, nil
}
