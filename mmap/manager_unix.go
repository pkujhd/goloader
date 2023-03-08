//go:build freebsd || linux || netbsd
// +build freebsd linux netbsd

package mmap

import (
	"bytes"
	"fmt"
	"github.com/eh-steve/goloader/mmap/mapping"
	"os"
	"strconv"
	"strings"
)

// Based on format of /proc/[pid]/maps from https://man7.org/linux/man-pages/man5/proc.5.html
// Only tested on linux, but netbsd/freebsd should be the same.
// TODO: OpenBSD needs to use procmap https://man.openbsd.org/procmap.1
// TODO: Solaris needs to use /proc/[pid]/map with a different format
// TODO: Dragonfly needs  /proc/curproc/map with a different format
func getCurrentProcMaps() ([]mapping.Mapping, error) {
	mapsData, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return nil, fmt.Errorf("could not read '/proc/self/maps': %w", err)
	}
	lines := bytes.Split(mapsData, []byte("\n"))
	var mappings []mapping.Mapping
	for i, line := range lines {
		var mapping mapping.Mapping
		mmapFields := strings.Fields(string(line))
		if len(mmapFields) == 0 {
			continue
		}
		if len(mmapFields) < 4 {
			return nil, fmt.Errorf("got fewer than 4 fields on line %d of /proc/self/maps: %s", i, line)
		}
		addrRange := strings.Split(mmapFields[0], "-")
		if len(addrRange) != 2 {
			return nil, fmt.Errorf("got %d fields for address range on line %d (expected 2): %s", len(addrRange), i, line)
		}
		startAddr, err := strconv.ParseUint(addrRange[0], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start address (%s) on line %d as uint64 (line: %s): %w", addrRange[0], i, line, err)
		}
		endAddr, err := strconv.ParseUint(addrRange[1], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse end address (%s) on line %d as uint64 (line: %s): %w", addrRange[1], i, line, err)
		}
		mapping.StartAddr = uintptr(startAddr)
		mapping.EndAddr = uintptr(endAddr)
		perms := mmapFields[1]
		for _, char := range perms {
			switch char {
			case 'r':
				mapping.ReadPerm = true
			case 'w':
				mapping.WritePerm = true
			case 'x':
				mapping.ExecutePerm = true
			case 's':
				mapping.SharedPerm = true
			case 'p':
				mapping.PrivatePerm = true
			case '-':
			default:
				return nil, fmt.Errorf("got an unexpected permission bit '%s' in perms '%s'", string(char), perms)
			}
		}
		offset, err := strconv.ParseUint(mmapFields[2], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse file offset (%s) on line %d as uint64 (line: %s): %w", mmapFields[2], i, line, err)
		}
		mapping.Offset = uintptr(offset)
		mapping.Dev = mmapFields[3]
		inode, err := strconv.ParseUint(mmapFields[4], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse inode (%s) on line %d as uint64 (line: %s): %w", mmapFields[4], i, line, err)
		}
		mapping.Inode = inode
		if len(mmapFields) > 5 {
			mapping.PathName = mmapFields[5]
		}
		mappings = append(mappings, mapping)
	}

	return mappings, nil
}
