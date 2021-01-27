// +build go1.8
// +build !go1.9

package goloader

import (
	"cmd/objfile/goobj"
)

const (
	R_PCREL = 15
	// R_TLS_LE, used on 386, amd64, and ARM, resolves to the offset of the
	// thread-local symbol from the thread local base and is used to implement the
	// "local exec" model for tls access (r.Sym is not set on intel platforms but is
	// set to a TLS symbol -- runtime.tlsg -- in the linker when externally linking).
	R_TLS_LE = 16
	// R_METHODOFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	// It is a variant of R_ADDROFF used when linking from the uncommonType of a
	// *rtype, and may be set to zero by the linker if it determines the method
	// text is unreachable by the linked program.
	R_METHODOFF = 24
)

const (
	Sxxx = iota
	STEXT
	SELFRXSECT
	STYPE
	SSTRING
	SGOSTRING
	SGOFUNC
	SGCBITS
	SRODATA
	SFUNCTAB
	SELFROSECT
	SMACHOPLT
	STYPERELRO
	SSTRINGRELRO
	SGOSTRINGRELRO
	SGOFUNCRELRO
	SGCBITSRELRO
	SRODATARELRO
	SFUNCTABRELRO
	STYPELINK
	SITABLINK
	SSYMTAB
	SPCLNTAB
	SELFSECT
	SMACHO
	SMACHOGOT
	SWINDOWS
	SELFGOT
	SNOPTRDATA
	SINITARR
	SDATA
	SBSS
	SNOPTRBSS
	STLSBSS
	SXREF
	SMACHOSYMSTR
	SMACHOSYMTAB
	SMACHOINDIRECTPLT
	SMACHOINDIRECTGOT
	SFILE
	SFILEPATH
	SCONST
	SDYNIMPORT
	SHOSTOBJ
	SDWARFSECT
	SDWARFINFO

	//not used, only adapter golang 1.16
	R_USEIFACE       = 0x10000000 - 3
	R_USEIFACEMETHOD = 0x10000000 - 2
	R_ADDRCUOFF      = 0x10000000 - 1
)

func addStackObject(codereloc *CodeReloc, funcname string, symbolMap map[string]uintptr) (err error) {
	return nil
}

func addDeferReturn(codereloc *CodeReloc, _func *_func) (err error) {
	return nil
}

type inlinedCall struct{}

func initInline(objFunc *goobj.Func, Func *FuncInfo, pkgpath string, fd *readAtSeeker) (err error) {
	return nil
}

func addInlineTree(codereloc *CodeReloc, _func *_func, objsym *ObjSymbol) (err error) {
	return nil
}

func _buildModule(codereloc *CodeReloc, codeModule *CodeModule) {
	codeModule.module.filetab = codereloc.filetab
}
