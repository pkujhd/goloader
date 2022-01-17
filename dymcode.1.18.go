//go:build go1.18 && !go1.19
// +build go1.18,!go1.19

package goloader

import (
	"unsafe"
)

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	module := codeModule.module
	module.pcHeader = (*pcHeader)(unsafe.Pointer(&(module.pclntable[0])))
	module.pcHeader.textStart = module.text
	module.pcHeader.nfunc = len(module.ftab)
	module.pcHeader.nfiles = (uint)(len(module.filetab))
	module.funcnametab = module.pclntable
	module.pctab = module.pclntable
	module.cutab = linker.filetab
	module.filetab = module.pclntable
	module.gofunc = module.noptrdata
	module.rodata = module.noptrdata
}
