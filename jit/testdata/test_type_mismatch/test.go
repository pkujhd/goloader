package test_type_mismatch

import "github.com/eh-steve/goloader/jit/testdata/test_type_mismatch/typedef"

func New() *typedef.Thing {
	// runtime.newobject will allocate an object according to the size of the _type in the relocation
	t := &typedef.Thing{}
	// The compiler will generate a MOV instruction with a fixed offset according to the size of the
	// type at compile time - this may not be the same as the above allocation
	t.C[len(t.C)-1] = 99
	return t
}
