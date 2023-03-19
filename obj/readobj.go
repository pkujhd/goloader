package obj

import (
	"cmd/objfile/goobj"
	"os"
)

type CompilationUnitFiles struct {
	ArchiveName string
	Files       []string
}

type Pkg struct {
	Syms              map[string]*ObjSymbol
	CUFiles           []CompilationUnitFiles
	Arch              string
	PkgPath           string
	F                 *os.File
	SymNameOrder      []string
	Objidx            uint32 // index of this archive in the slice of files
	ReferencedPkgs    []string
	UnresolvedSymRefs map[goobj.SymRef]struct{}
	SymNamesByIdx     map[uint32]string
}

type FuncInfo struct {
	Args      uint32
	Locals    uint32
	FuncID    uint8
	FuncFlag  uint8
	StartLine int32
	PCSP      []byte
	PCFile    []byte
	PCLine    []byte
	PCInline  []byte
	PCData    [][]byte
	File      []string
	FuncData  []string
	InlTree   []InlTreeNode
	ABI       uint16
	CUOffset  int
}

type ObjSymbol struct {
	Name   string
	Kind   int    // kind of symbol
	DupOK  bool   // are duplicate definitions okay?
	Size   int64  // size of corresponding data
	Data   []byte // memory image of symbol
	Type   string
	Reloc  []Reloc
	Func   *FuncInfo // additional data for functions
	Objidx uint32    // the index of the archive which the symbol came from when loading multiple files
}

type InlTreeNode struct {
	Parent   int64
	File     string
	Line     int64
	Func     string
	ParentPC int64
}

type Func struct {
	PCData   []uint32
	FuncData []uintptr
}

// copy from $GOROOT/src/cmd/internal/goobj/read.go type Sym struct
type Sym struct {
	Name   string
	Kind   int
	Offset int
	Func   *Func
	Reloc  []Reloc
}

// copy from $GOROOT/src/cmd/internal/goobj/read.go type Reloc struct
type Reloc struct {
	Offset         int
	Sym            *Sym
	Size           int
	Type           int
	Add            int
	EpilogueOffset int // This is added to store the offset of the extra instructions added by goloader in the case of certain overflowing relocations, e.g. ADRP, PCREL, CALLARM64
	EpilogueSize   int
}
