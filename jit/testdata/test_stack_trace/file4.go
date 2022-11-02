package test_stack_trace

import (
	"github.com/pkujhd/goloader/jit/testdata/common"
)

//go:noinline
func (m *SomeType) callSite4(msg common.SomeStruct) {

	// ARSE

	// ARSE

	// ARSE

	panic(msg.Val1)
}
