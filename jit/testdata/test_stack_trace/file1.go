package test_stack_trace

import "github.com/eh-steve/goloader/jit/testdata/common"

//go:noinline
func (m *SomeType) callSite1(msg common.SomeStruct) {
	m.callSite2(msg)
}
