//go:build go1.12 && !go1.14
// +build go1.12,!go1.14

package goloader

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	codeModule.module.filetab = linker.filetab
	codeModule.module.hasmain = 0
}
