//go:build go1.16 && !go1.23
// +build go1.16,!go1.23

package obj

import (
	"cmd/objfile/archive"
	"cmd/objfile/goobj"
	"cmd/objfile/objabi"
	"fmt"
	"os"
	"path/filepath"
)

func (pkg *Pkg) Symbols() error {
	file, err := os.Open(pkg.File)
	if err != nil {
		return err
	}
	defer file.Close()
	a, err := archive.Parse(file, false)
	if err != nil {
		return err
	}
	for _, e := range a.Entries {
		switch e.Type {
		case archive.EntryPkgDef:
			//nothing todo
		case archive.EntryGoObj:
			b := make([]byte, e.Obj.Size)
			_, err := file.ReadAt(b, e.Obj.Offset)
			if err != nil {
				return err
			}
			r := goobj.NewReaderFromBytes(b, false)
			// Name of referenced indexed symbols.
			nrefName := r.NRefName()
			refNames := make(map[goobj.SymRef]string, nrefName)
			for i := 0; i < nrefName; i++ {
				rn := r.RefName(i)
				refNames[rn.Sym()] = rn.Name(r)
			}
			for i := 0; i < r.NFile(); i++ {
				pkg.CUFiles = append(pkg.CUFiles, r.File(i))
			}
			pkg.Arch = e.Obj.Arch
			nsym := r.NSym() + r.NHashed64def() + r.NHasheddef() + r.NNonpkgdef()
			for i := 0; i < nsym; i++ {
				pkg.addSym(r, uint32(i), &refNames)
			}
			for _, importPkg := range r.Autolib() {
				path := importPkg.Pkg
				path = path[:len(path)-len(filepath.Ext(path))]
				pkg.ImportPkgs = append(pkg.ImportPkgs, path)
			}
		default:
			return fmt.Errorf("Parse open %s: unrecognized archive member %s\n", file.Name(), e.Name)
		}
	}
	return nil
}

func resolveSymRef(s goobj.SymRef, r *goobj.Reader, refNames *map[goobj.SymRef]string) (string, uint32) {
	i := InvalidIndex
	switch p := s.PkgIdx; p {
	case goobj.PkgIdxInvalid:
		if s.SymIdx != 0 {
			panic("bad sym ref")
		}
		return EmptyString, i
	case goobj.PkgIdxHashed64:
		i = s.SymIdx + uint32(r.NSym())
	case goobj.PkgIdxHashed:
		i = s.SymIdx + uint32(r.NSym()+r.NHashed64def())
	case goobj.PkgIdxNone:
		i = s.SymIdx + uint32(r.NSym()+r.NHashed64def()+r.NHasheddef())
	case goobj.PkgIdxBuiltin:
		name, _ := goobj.BuiltinName(int(s.SymIdx))
		return name, i
	case goobj.PkgIdxSelf:
		i = s.SymIdx
	default:
		return (*refNames)[s], i
	}
	return r.Sym(i).Name(r), i
}

func (pkg *Pkg) addSym(r *goobj.Reader, index uint32, refNames *map[goobj.SymRef]string) {
	s := r.Sym(index)
	symbol := ObjSymbol{Name: s.Name(r), Kind: int(s.Type()), DupOK: s.Dupok(), Size: (int64)(s.Siz()), Func: &FuncInfo{}}
	if _, ok := pkg.Syms[symbol.Name]; ok {
		return
	}
	if objabi.SymKind(symbol.Kind) == objabi.Sxxx || symbol.Name == EmptyString {
		return
	}
	if symbol.Size > 0 {
		symbol.Data = r.Data(index)
		grow(&symbol.Data, (int)(symbol.Size))
	} else {
		symbol.Data = make([]byte, 0)
	}

	pkg.Syms[symbol.Name] = &symbol

	auxs := r.Auxs(index)
	for k := 0; k < len(auxs); k++ {
		name, index := resolveSymRef(auxs[k].Sym(), r, refNames)
		switch auxs[k].Type() {
		case goobj.AuxGotype:
			symbol.Type = name
		case goobj.AuxFuncInfo:
			funcInfo := goobj.FuncInfo{}
			readFuncInfo(&funcInfo, r.Data(index), symbol.Func)
			symbol.Func.CUOffset = 0
			for _, index := range funcInfo.File {
				symbol.Func.File = append(symbol.Func.File, r.File(int(index)))
			}
			for _, inl := range funcInfo.InlTree {
				funcname, _ := resolveSymRef(inl.Func, r, refNames)
				funcname = ReplacePkgPath(funcname, pkg.PkgPath)
				inlNode := InlTreeNode{
					Parent:   int64(inl.Parent),
					File:     r.File(int(inl.File)),
					Line:     int64(inl.Line),
					Func:     funcname,
					ParentPC: int64(inl.ParentPC),
				}
				symbol.Func.InlTree = append(symbol.Func.InlTree, inlNode)
			}
		case goobj.AuxFuncdata:
			symbol.Func.FuncData = append(symbol.Func.FuncData, name)
		case goobj.AuxDwarfInfo:
		case goobj.AuxDwarfLoc:
		case goobj.AuxDwarfRanges:
		case goobj.AuxDwarfLines:
		case goobj.AuxPcsp:
			symbol.Func.PCSP = r.Data(index)
		case goobj.AuxPcfile:
			symbol.Func.PCFile = r.Data(index)
		case goobj.AuxPcline:
			symbol.Func.PCLine = r.Data(index)
		case goobj.AuxPcinline:
			symbol.Func.PCInline = r.Data(index)
		case goobj.AuxPcdata:
			symbol.Func.PCData = append(symbol.Func.PCData, r.Data(index))
		}
		if _, ok := pkg.Syms[name]; !ok && index != InvalidIndex {
			pkg.addSym(r, index, refNames)
		}
	}

	relocs := r.Relocs(index)
	for k := 0; k < len(relocs); k++ {
		symbol.Reloc = append(symbol.Reloc, Reloc{})
		symbol.Reloc[k].Add = int(relocs[k].Add())
		symbol.Reloc[k].Offset = int(relocs[k].Off())
		symbol.Reloc[k].Size = int(relocs[k].Siz())
		symbol.Reloc[k].Type = int(relocs[k].Type())
		name, index := resolveSymRef(relocs[k].Sym(), r, refNames)
		symbol.Reloc[k].SymName = name
		if _, ok := pkg.Syms[name]; !ok && index != InvalidIndex {
			pkg.addSym(r, index, refNames)
		}
	}
}
