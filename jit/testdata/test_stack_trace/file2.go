package test_stack_trace

import (
	"github.com/pkujhd/goloader/jit/testdata/common"
)

//go:noinline
func (m *SomeType) callSite2(msg common.SomeStruct) {

	// ARSE
	m.callSite3(msg)
}
