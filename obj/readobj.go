package obj

import (
	"os"
)

type Pkg struct {
	Syms    map[string]*ObjSymbol
	Arch    string
	PkgPath string
	File    *os.File
	CUFiles []string
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
	CUOffset  int32
}

type ObjSymbol struct {
	Name  string
	Kind  int    // kind of symbol
	DupOK bool   // are duplicate definitions okay?
	Size  int64  // size of corresponding data
	Data  []byte // memory image of symbol
	Type  string
	Reloc []Reloc
	Func  *FuncInfo // additional data for functions
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
	Offset int
	Sym    *Sym
	Size   int
	Type   int
	Add    int
	Epilogue
}

type Epilogue struct {
	Offset int
	Size   int
}
