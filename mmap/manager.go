package mmap

import (
	"fmt"
	"github.com/pkujhd/goloader/mmap/mapping"
	"math"
	"sort"
	"sync"
	"syscall"
	"unsafe"
)

var pageSize = uintptr(syscall.Getpagesize())

func roundPageUp(p uintptr) uintptr {
	return (p & ^(pageSize - 1)) + pageSize
}

func roundPageDown(p uintptr) uintptr {
	return p & ^(pageSize - 1)
}

type gap struct {
	startAddr uintptr
	endAddr   uintptr
}

func findNextFreeAddressesAfterTarget(targetAddr uintptr, size int, mappings []mapping.Mapping) (gaps []gap, err error) {
	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].StartAddr < mappings[j].StartAddr
	})

	var allGaps []gap
	for i, mapping := range mappings[:len(mappings)-1] {
		allGaps = append(allGaps, gap{
			startAddr: mapping.EndAddr,
			endAddr:   roundPageDown(mappings[i+1].StartAddr),
		})
	}
	allGaps = append(allGaps, gap{
		startAddr: mappings[len(mappings)-1].EndAddr,
		endAddr:   math.MaxUint64, // We really shouldn't be in this situation...
	})
	var suitableGaps []gap
	for _, g := range allGaps {
		if g.startAddr > targetAddr && int(g.endAddr-g.startAddr) >= size {
			suitableGaps = append(suitableGaps, g)
		}
	}
	if len(suitableGaps) == 0 {
		return suitableGaps, fmt.Errorf("could not find free address range with size 0x%x after target 0x%x", size, targetAddr)
	}
	return suitableGaps, nil
}

//go:linkname activeModules runtime.activeModules
func activeModules() []unsafe.Pointer

// This isn't concurrency safe since other code outside goloader might mmap something in the same region we're trying to.
// Ideally we would just skip these collisions via MAP_FIXED_NOREPLACE but this isn't portable...
var mmapLock sync.Mutex

func AcquireMapping(size int, mapFunc func(size int, addr uintptr) ([]byte, error)) ([]byte, error) {
	mmapLock.Lock()
	defer mmapLock.Unlock()

	firstModuleAddr := uintptr(activeModules()[0])
	mappings, err := getCurrentProcMaps()
	if err != nil {
		return nil, err
	}

	gaps, err := findNextFreeAddressesAfterTarget(firstModuleAddr, int(roundPageUp(uintptr(size))), mappings)
	if err != nil {
		return nil, err
	}

	for _, gap := range gaps {
		start := gap.startAddr
		end := gap.endAddr
		for i := start; i+roundPageUp(uintptr(size)) < end; i += roundPageUp(uintptr(size)) {
			mapping, err := mapFunc(int(roundPageUp(uintptr(size))), i)
			if err != nil {
				// Keep going, try again
			} else {
				if uintptr(unsafe.Pointer(&mapping[len(mapping)-1]))-firstModuleAddr > 1<<32 {
					err = mapper.Munmap(mapping)
					if err != nil {
						return nil, fmt.Errorf("failed to acquire a mapping within 32 bits of the first module address, wanted 0x%x, got %p - %p, also failed to munmap: %w", firstModuleAddr, &mapping[0], &mapping[len(mapping)-1], err)
					}
					return nil, fmt.Errorf("failed to acquire a mapping within 32 bits of the first module address, wanted 0x%x, got %p - %p", firstModuleAddr, &mapping[0], &mapping[len(mapping)-1])
				}
				return mapping, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to aquire mapping between taken mappings: \n%s", formatTakenMappings(mappings))
}

func formatTakenMappings(mappings []mapping.Mapping) string {
	var mappingTakenRanges string
	for _, m := range mappings {
		mappingTakenRanges += fmt.Sprintf("0x%x - 0x%x  %s\n", m.StartAddr, m.EndAddr, m.PathName)
	}
	return mappingTakenRanges
}
