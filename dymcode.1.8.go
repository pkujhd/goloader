//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package goloader

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	codeModule.module.filetab = linker.filetab
}
