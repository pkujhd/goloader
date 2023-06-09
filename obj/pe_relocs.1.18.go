//go:build go1.18 && !go1.19
// +build go1.18,!go1.19

package obj

import (
	"cmd/objfile/archive"
	"debug/pe"
	"fmt"
)

func (pkg *Pkg) convertPERelocs(f *pe.File, e archive.Entry) error {
	return fmt.Errorf("no support for PE relocs on go1.18 (yet)")
}
