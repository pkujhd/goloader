package test_complex_func

import (
	"context"
	"fmt"
	"github.com/eh-steve/goloader/jit/testdata/common"
	"net"
	"time"
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
	d := net.Dialer{
		Deadline: time.Now().Add(-10 * time.Second),
	}
	_, err := d.DialContext(context.Background(), "tcp", "localhost:9999")
	if err != nil {
		// Force access to a heap string ("i/o timeout") accessed via a PCREL ADD relocation on amd64
		fmt.Println(err.Error())
	}
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
