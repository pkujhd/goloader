//go:build darwin
// +build darwin

package mmap

import (
	"github.com/eh-steve/goloader/mmap/mapping"
	"github.com/eh-steve/goloader/mmap/vmmap"
)

// https://developer.apple.com/library/archive/documentation/Performance/Conceptual/ManagingMemory/Articles/VMPages.html
func getCurrentProcMaps() ([]mapping.Mapping, error) {
	return vmmap.Vmmap()
}
