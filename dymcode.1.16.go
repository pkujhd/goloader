//go:build go1.16 && !go1.17
// +build go1.16,!go1.17

package goloader

import (
	"unsafe"
)

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	module := codeModule.module
	module.pcHeader = (*pcHeader)(unsafe.Pointer(&(module.pclntable[0])))
	module.pcHeader.nfunc = len(module.ftab)
	module.pcHeader.nfiles = (uint)(len(module.filetab))
	module.funcnametab = module.pclntable
	module.pctab = module.pclntable
	module.cutab = linker.filetab
	module.filetab = module.pclntable
	module.hasmain = 0
}
