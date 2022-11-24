//go:build darwin && vmmap_fallback
// +build darwin,vmmap_fallback

package vmmap

import (
	"fmt"
	"github.com/pkujhd/goloader/mmap/mapping"
	"os"
	"os/exec"
)

func Vmmap() ([]mapping.Mapping, error) {
	// TODO - actually implement parsing the vmmap output as a fallback in case
	pid := os.Getpid()
	cmd := exec.Command("vmmap", "-pages", "-interleaved", fmt.Sprintf("%d", pid))
	output, err := cmd.CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("could not run 'vmmap -v %d': %w", pid, err)
	}
	panic("Parsing this horrible and slow output not implemented yet:" + string(output))
}
