// +build go1.12
// +build !go1.14

package goloader

import (
	"cmd/objfile/goobj"
	"strings"
)

// copy from $GOROOT/src/cmd/internal/objabi/reloctype.go
const (
	R_ADDR = 1
	// R_ADDRARM64 relocates an adrp, add pair to compute the address of the
	// referenced symbol.
	R_ADDRARM64 = 3
	// R_ADDROFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	R_ADDROFF = 5
	// R_WEAKADDROFF resolves just like R_ADDROFF but is a weak relocation.
	// A weak relocation does not make the symbol it refers to reachable,
	// and is only honored by the linker if the symbol is in some other way
	// reachable.
	R_WEAKADDROFF = 6
	R_CALL        = 8
	R_CALLARM     = 9
	R_CALLARM64   = 10
	R_CALLIND     = 11
	R_PCREL       = 15
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

	//not used, only adapter golang 1.16
	R_USETYPE        = 0x10000000 - 4
	R_USEIFACE       = 0x10000000 - 3
	R_USEIFACEMETHOD = 0x10000000 - 2
	R_ADDRCUOFF      = 0x10000000 - 1
	R_WEAKADDR       = 0x20000000
)

// copy from $GOROOT/src/cmd/internal/objabi/symkind.go
const (
	// An otherwise invalid zero value for the type
	Sxxx = iota
	// Executable instructions
	STEXT
	// Read only static data
	SRODATA
	// Static data that does not contain any pointers
	SNOPTRDATA
	// Static data
	SDATA
	// Statically data that is initially all 0s
	SBSS
	// Statically data that is initially all 0s and does not contain pointers
	SNOPTRBSS
	// Thread-local data that is initially all 0s
	STLSBSS
	// Debugging data
	SDWARFINFO
	SDWARFRANGE
	SDWARFLOC
	SDWARFMISC
	// ABI alias. An ABI alias symbol is an empty symbol with a
	// single relocation with 0 size that references the native
	// function implementation symbol.
	//
	// TODO(austin): Remove this and all uses once the compiler
	// generates real ABI wrappers rather than symbol aliases.
	SABIALIAS
	// Update cmd/link/internal/sym/AbiSymKindToSymKind for new SymKind values.
)

func (linker *Linker) addStackObject(funcname string, symbolMap map[string]uintptr) (err error) {
	return linker._addStackObject(funcname, symbolMap)
}

func (linker *Linker) addDeferReturn(_func *_func) (err error) {
	return nil
}

// inlinedCall is the encoding of entries in the FUNCDATA_InlTree table.
type inlinedCall struct {
	parent   int16  // index of parent in the inltree, or < 0
	funcID   funcID // type of the called function
	_        byte
	file     int32 // fileno index into filetab
	line     int32 // line number of the call site
	func_    int32 // offset into pclntab for name of called function
	parentPc int32 // position of an instruction whose source position is the call site (offset from entry)
}

func (linker *Linker) initInlinedCall(inl InlTreeNode, _func *_func) inlinedCall {
	return inlinedCall{
		parent:   int16(inl.Parent),
		funcID:   _func.funcID,
		file:     findFileTab(linker, inl.File),
		line:     int32(inl.Line),
		func_:    int32(linker.namemap[inl.Func]),
		parentPc: int32(inl.ParentPC)}
}

func initInline(objFunc *goobj.Func, Func *FuncInfo, pkgpath string, fd *readAtSeeker) (err error) {
	for _, inl := range objFunc.InlTree {
		inline := InlTreeNode{
			Parent:   int64(inl.Parent),
			File:     inl.File,
			Line:     int64(inl.Line),
			Func:     inl.Func.Name,
			ParentPC: int64(inl.ParentPC),
		}
		inline.Func = strings.Replace(inline.Func, EmptyPkgPath, pkgpath, -1)
		Func.InlTree = append(Func.InlTree, inline)
	}
	Func.PCInline, err = fd.BytesAt(objFunc.PCInline.Offset, objFunc.PCInline.Size)
	return err
}

func (linker *Linker) addInlineTree(_func *_func, objsym *ObjSymbol) (err error) {
	return linker._addInlineTree(_func, objsym)
}

func (linker *Linker) _buildModule(codeModule *CodeModule) {
	codeModule.module.filetab = linker.filetab
	codeModule.module.hasmain = 0
}
