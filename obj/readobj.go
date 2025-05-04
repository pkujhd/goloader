package obj

type Pkg struct {
	Syms       map[string]*ObjSymbol
	CgoImports map[string]*CgoImport
	GoArchive  *Archive
	SymIndex   []string
	Arch       string
	PkgPath    string
	File       string
	ImportPkgs []string
	CUFiles    []string
	CUOffset   int32
}

type CgoImport struct {
	GoSymName string
	CSymName  string
	SoName    string
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
	ABI   uint
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
	Type   string
	Kind   int
	Offset int
	Func   *Func
	Reloc  []Reloc
}

// copy from $GOROOT/src/cmd/internal/goobj/read.go type Reloc struct
type Reloc struct {
	Offset  int
	SymName string
	Size    int
	Type    int
	Add     int
	Instruction
	Epilogue
}

type Instruction struct {
	Op    uint
	Start int
	End   int
	Text  []byte
	Args  []string
}

type Epilogue struct {
	Offset int
	Size   int
}

func (r *Reloc) GetStart() int {
	if r.Start != 0 {
		return r.Start
	}
	return r.Offset
}

func (r *Reloc) GetEnd() int {
	if r.End != 0 {
		return r.End
	}
	return r.Offset + r.Size
}
