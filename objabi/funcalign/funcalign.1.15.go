//go:build go1.15 && !go1.16
// +build go1.15,!go1.16

package funcalign

import (
	"cmd/objfile/sys"
	"fmt"
)

func GetFuncAlign(arch *sys.Arch) int {
	switch arch.Name {
	//see:^cmd/linker/internal/arm/l.go
	case sys.ArchARM.Name:
		return 4
	//see:^cmd/linker/internal/arm64/l.go
	case sys.ArchARM64.Name:
		return 16
	//see:^cmd/linker/internal/x86/l.go
	case sys.Arch386.Name:
		return 16
	// see:^cmd/linker/internal/amd64/l.go
	case sys.ArchAMD64.Name:
		return 16
	default:
		panic(fmt.Errorf("not support arch:%s", arch.Name))
	}
}
