package test_stack_trace

import (
	. "github.com/eh-steve/goloader/jit/testdata/common"
)

type SomeType struct {
}

func NewThing() SomeInterface {
	return &SomeType{}
}

func (m *SomeType) Method1(input SomeStruct) (SomeStruct, error) {
	m.callSite1(input)
	return input, nil
}

func (m *SomeType) Method2(input map[string]interface{}) error {
	panic("implement me")
}
