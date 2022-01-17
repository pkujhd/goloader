//go:build go1.14 && !go1.16
// +build go1.14,!go1.16

package goloader

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	codeModule.module.filetab = linker.filetab
	codeModule.module.hasmain = 0
}
