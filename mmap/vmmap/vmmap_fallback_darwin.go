//go:build darwin && !cgo
// +build darwin,!cgo

package vmmap

import (
	"bytes"
	"fmt"
	"github.com/eh-steve/goloader/mmap/mapping"
	"os"
	"os/exec"
	"strconv"
)

func Vmmap() ([]mapping.Mapping, error) {
	// This is horrible
	pid := os.Getpid()
	cmd := exec.Command("vmmap", "-pages", "-interleaved", fmt.Sprintf("%d", pid))
	output, err := cmd.CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("could not run 'vmmap -v %d': %w", pid, err)
	}

	sections := bytes.Split(output, []byte("REGION DETAIL\n"))
	lines := bytes.Split(sections[0], []byte("\n"))

	columnHeaders := append(lines[len(lines)-1], []byte("REGION DETAIL")...)
	if len(sections) != 2 {
		return nil, fmt.Errorf("failed to parse vmmap output: expected REGION_DETAIL to be column header, got %d", len(sections))
	}
	sections = bytes.Split(sections[1], []byte("==== Legend\n"))
	if len(sections) != 2 {
		return nil, fmt.Errorf("failed to parse vmmap output: expected REGION_DETAIL to be column header, got")
	}

	// The vmmap address start/end output is centered around a common hyphen placement - we need to find the index of that hyphen, then split around it
	hyphenIndex := bytes.Index(columnHeaders, []byte("START - END")) + len("START ")
	entries := bytes.Split(sections[0], []byte("\n"))
	var mappings = make([]mapping.Mapping, 0, len(entries))
	for rowNumber, entry := range entries {
		if len(entry) <= hyphenIndex {
			continue
		}
		var i int
		for i = hyphenIndex; i > 0; i-- {
			if entry[i] == ' ' {
				i++
				break
			}
		}
		startStr := string(entry[i:hyphenIndex])
		for i = hyphenIndex + 1; i < hyphenIndex+18; i++ {
			if entry[i] == ' ' {
				break
			}
		}
		endStr := string(entry[hyphenIndex+1 : i])
		start, err := strconv.ParseUint(startStr, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start hex uint64 from '%s' on row %d of vmmap output. Full output:\n%s", startStr, rowNumber, output)
		}
		end, err := strconv.ParseUint(endStr, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse end hex uint64 from '%s' on row %d of vmmap output. Full output:\n%s", endStr, rowNumber, output)
		}
		m := mapping.Mapping{
			StartAddr: uintptr(start),
			EndAddr:   uintptr(end),
		}
		mappings = append(mappings, m)
	}

	return mappings, nil
}
