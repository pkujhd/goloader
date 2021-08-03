package goloader

import "cmd/objfile/sys"

func getArch(archName string) *sys.Arch {
	arch := &sys.Arch{}
	for index := range sys.Archs {
		if archName == sys.Archs[index].Name {
			arch = sys.Archs[index]
		}
	}
	return arch
}
