// +build go1.16
// +build !go1.17

package goloader

import (
	"cmd/objfile/archive"
	"cmd/objfile/goobj"
	"cmd/objfile/objabi"
	"cmd/objfile/sys"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

//golang 1.16 change magic number
var (
	x86moduleHead = []byte{0xFA, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x1, PtrSize}
	armmoduleHead = []byte{0xFA, 0xFF, 0xFF, 0xFF, 0x0, 0x0, 0x4, PtrSize}
)

func Parse(f *os.File, pkgpath *string) ([]string, error) {
	a, err := archive.Parse(f, false)
	if err != nil {
		return nil, err
	}
	symbols := make([]string, 0)
	for _, e := range a.Entries {
		switch e.Type {
		case archive.EntryPkgDef:
			//nothing todo
		case archive.EntryGoObj:
			o := e.Obj
			b := make([]byte, o.Size)
			_, err := f.ReadAt(b, o.Offset)
			if err != nil {
				return nil, err
			}
			r := goobj.NewReaderFromBytes(b, false)
			nsym := r.NSym()
			for i := 0; i < nsym; i++ {
				if r.Sym(uint32(i)).Name(r) != EmptyString {
					symbols = append(symbols, r.Sym(uint32(i)).Name(r))
				}
			}
		default:
			return nil, fmt.Errorf("Parse open %s: unrecognized archive member %s", f.Name(), e.Name)
		}
	}
	return symbols, nil
}

func addPclntableHeader(reloc *CodeReloc) {
	head := make([]byte, unsafe.Sizeof(pcHeader{}))
	copy(head, x86moduleHead)
	reloc.pclntable = append(reloc.pclntable, head...)
}

func symbols(f *os.File, pkgpath string) (map[string]*ObjSymbol, string, error) {
	a, err := archive.Parse(f, false)
	if err != nil {
		return nil, EmptyString, err
	}
	objs := make(map[string]*ObjSymbol, 0)
	Arch := EmptyString
	for _, e := range a.Entries {
		switch e.Type {
		case archive.EntryPkgDef:
			//nothing todo
		case archive.EntryGoObj:
			o := e.Obj
			b := make([]byte, o.Size)
			_, err := f.ReadAt(b, o.Offset)
			if err != nil {
				return nil, EmptyString, err
			}
			r := goobj.NewReaderFromBytes(b, false)
			var arch *sys.Arch
			for _, a := range sys.Archs {
				if a.Name == e.Obj.Arch {
					arch = a
					break
				}
			}
			// Name of referenced indexed symbols.
			nrefName := r.NRefName()
			refNames := make(map[goobj.SymRef]string, nrefName)
			for i := 0; i < nrefName; i++ {
				rn := r.RefName(i)
				refNames[rn.Sym()] = rn.Name(r)
			}
			Arch = arch.Name
			nsym := r.NSym()
			for i := 0; i < nsym; i++ {
				AddSym(r, uint32(i), &pkgpath, &refNames, o, objs)
			}
		default:
			return nil, EmptyString, fmt.Errorf("Parse open %s: unrecognized archive member %s", f.Name(), e.Name)
		}
	}
	for _, sym := range objs {
		sym.Name = strings.Replace(sym.Name, EmptyPkgPath, pkgpath, -1)
	}
	return objs, Arch, nil
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

func AddSym(r *goobj.Reader, index uint32, pkgpath *string, refNames *map[goobj.SymRef]string, o *archive.GoObj, objs map[string]*ObjSymbol) {
	s := r.Sym(index)
	symbol := ObjSymbol{Name: s.Name(r), Kind: int(s.Type()), DupOK: s.Dupok(), Size: (int64)(s.Siz()), Func: &FuncInfo{}}
	if objabi.SymKind(symbol.Kind) == objabi.Sxxx || symbol.Name == EmptyString {
		return
	}
	if _, ok := objs[symbol.Name]; ok {
		return
	}
	if symbol.Size > 0 {
		symbol.Data = r.Data(index)
		grow(&symbol.Data, (int)(symbol.Size))
	} else {
		symbol.Data = make([]byte, 0)
	}

	objs[symbol.Name] = &symbol

	auxs := r.Auxs(index)
	for k := 0; k < len(auxs); k++ {
		name, index := resolveSymRef(auxs[k].Sym(), r, refNames)
		switch auxs[k].Type() {
		case goobj.AuxGotype:
		case goobj.AuxFuncInfo:
			funcInfo := goobj.FuncInfo{}
			funcInfo.Read(r.Data(index))
			symbol.Func.Args = funcInfo.Args
			symbol.Func.Locals = funcInfo.Locals
			symbol.Func.FuncID = (uint8)(funcInfo.FuncID)
			for _, index := range funcInfo.File {
				symbol.Func.File = append(symbol.Func.File, r.File(int(index)))
			}
			for _, inl := range funcInfo.InlTree {
				funcname, _ := resolveSymRef(inl.Func, r, refNames)
				funcname = strings.Replace(funcname, EmptyPkgPath, *pkgpath, -1)
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
		if _, ok := objs[name]; !ok && index != InvalidIndex {
			AddSym(r, index, pkgpath, refNames, o, objs)
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
		symbol.Reloc[k].Sym = &Sym{Name: name, Offset: InvalidOffset}
		if _, ok := objs[name]; !ok && index != InvalidIndex {
			AddSym(r, index, pkgpath, refNames, o, objs)
		}
	}
}
