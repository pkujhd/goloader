//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package goloader

import (
	"cmd/objfile/goobj"
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
)

func (linker *Linker) addStackObject(funcname string, symbolMap map[string]uintptr, module *moduledata) (err error) {
	return nil
}

func (linker *Linker) addDeferReturn(_func *_func) (err error) {
	return nil
}

type inlinedCall struct{}

func initInline(objFunc *goobj.Func, Func *FuncInfo, pkgpath string, fd *readAtSeeker) (err error) {
	return nil
}

func (linker *Linker) addInlineTree(_func *_func, objsym *ObjSymbol) (err error) {
	return nil
}

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	codeModule.module.filetab = linker.filetab
}
