package test_stack_split

import (
	"fmt"
	"unsafe"
)

func RecurseUntilMaxDepth(depth int, oldAddr, prevDiff uintptr, splitCount int) int {
	const maxDepth = 700000 // (Around 100MB of stack)
	if depth > maxDepth {
		return splitCount
	}
	var someVarOnStack int
	addr := uintptr(unsafe.Pointer(&someVarOnStack))
	diff := oldAddr - addr
	if diff != prevDiff {
		fmt.Printf("Possible stack split Old: 0x%x New: 0x%x Diff: %d\n", oldAddr, addr, int64(diff-prevDiff))
		splitCount = splitCount + 1
		diff = prevDiff
	}
	return RecurseUntilMaxDepth(depth+1, addr, diff, splitCount)
}
