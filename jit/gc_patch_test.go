package jit

import "testing"

func TestPatch(t *testing.T) {
	err := PatchGC("go", true)
	if err != nil {
		t.Fatal(err)
	}
}
