//go:build go1.9 && !go1.12
// +build go1.9,!go1.12

package goloader

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	codeModule.module.filetab = linker.filetab
}
