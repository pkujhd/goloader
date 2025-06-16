//go:build go1.16 && !go1.26
// +build go1.16,!go1.26

package obj

import (
	"cmd/objfile/archive"
	"cmd/objfile/goobj"
	"cmd/objfile/obj"
	"cmd/objfile/objabi"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkujhd/goloader/objabi/symkind"
)

func encodeSymRef(symref goobj.SymRef) string {
	return fmt.Sprintf(UNRESOLVED_SYMREF_FMT, int(symref.PkgIdx), int(symref.SymIdx))
}

func resolveSymRefName(refname string, packages map[string]*Pkg, pkgPath string, entryId int, cache map[string]string) string {
	if !strings.HasPrefix(refname, UNRESOLVED_SYMREF_PREFIX) {
		return refname
	}
	if name, ok := cache[refname]; ok {
		return name
	}
	cache[refname] = EmptyString
	splits := strings.Split(refname, "#")
	pkgidx, _ := strconv.Atoi(splits[1])
	symidx, _ := strconv.Atoi(splits[2])
	ref := goobj.SymRef{PkgIdx: uint32(pkgidx), SymIdx: uint32(symidx)}
	goArchive := packages[pkgPath].GoArchive
	r := goArchive.entries[entryId].r
	symIndex := packages[pkgPath].SymIndex[goArchive.entries[entryId].startIndex:]
	index := symRef2Index(ref, r)
	if index == InvalidIndex {
		if ref.PkgIdx == goobj.PkgIdxInvalid && ref.SymIdx == 0 {
			cache[refname] = EmptyString
		} else if ref.PkgIdx == goobj.PkgIdxBuiltin {
			builtinName, _ := goobj.BuiltinName(int(ref.SymIdx))
			cache[refname] = builtinName
		} else if name, ok := goArchive.refNames[ref]; ok {
			cache[refname] = name
		} else if ref.PkgIdx < goobj.PkgIdxSelf {
			if ref.PkgIdx >= uint32(r.NPkg()) {
				cache[refname] = EmptyString
			} else {
				nPkgPath := r.Pkg(int(ref.PkgIdx))
				if nPkg, ok := packages[nPkgPath]; ok {
					cache[refname] = nPkg.SymIndex[ref.SymIdx]
					return nPkg.SymIndex[ref.SymIdx]
				} else {
					cache[refname] = EmptyString
				}
			}
		}
	} else {
		cache[refname] = symIndex[index]
	}
	return cache[refname]
}

type symIndex struct {
	entryIndex  int
	symbolIndex int
}

type entry struct {
	r          *goobj.Reader
	syms       []*ObjSymbol
	startIndex int
}

type Archive struct {
	symVersions []map[string]symIndex
	refNames    map[goobj.SymRef]string
	entryId     int
	entries     []entry
}

func (v Archive) MarshalBinary() ([]byte, error) {
	return make([]byte, 0), nil
}

func (v *Archive) UnmarshalBinary(data []byte) error {
	return nil
}

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
	goArchive := Archive{
		symVersions: make([]map[string]symIndex, 0),
		refNames:    make(map[goobj.SymRef]string, 0),
		entryId:     0,
	}
	pkg.GoArchive = &goArchive
	for i := 0; i < int(obj.ABICount); i++ {
		goArchive.symVersions = append(goArchive.symVersions, make(map[string]symIndex))
	}
	for _, e := range a.Entries {
		switch e.Type {
		case archive.EntryPkgDef:
			//nothing todo
		case archive.EntryGoObj:
			pkg.Arch = e.Obj.Arch
			fields := strings.Fields(string(e.Obj.TextHeader))
			for index, field := range fields {
				if field == "cgo" {
					var cgo_imports [][]string
					json.NewDecoder(strings.NewReader(fields[index+1])).Decode(&cgo_imports)
					for _, cgo_import := range cgo_imports {
						switch cgo_import[0] {
						case "cgo_import_dynamic":
							pkg.CgoImports[cgo_import[1]] = &CgoImport{cgo_import[1], cgo_import[2], cgo_import[3]}
						case "cgo_import_static":
						case "cgo_export_dynamic":
						case "cgo_export_static":
						case "cgo_ldflag":
							//nothing
						}
					}
				}
			}
			b := make([]byte, e.Obj.Size)
			_, err := file.ReadAt(b, e.Obj.Offset)
			if err != nil {
				return err
			}
			r := goobj.NewReaderFromBytes(b, false)
			goArchive.entries = append(goArchive.entries, entry{r: r, syms: make([]*ObjSymbol, 0)})
			// Name of referenced indexed symbols.
			for i := 0; i < r.NRefName(); i++ {
				rn := r.RefName(i)
				goArchive.refNames[rn.Sym()] = rn.Name(r)
			}
			for i := 0; i < r.NFile(); i++ {
				pkg.CUFiles = append(pkg.CUFiles, r.File(i))
			}
			for i := 0; i < r.NSym()+r.NHashed64def()+r.NHasheddef()+r.NNonpkgdef()+r.NNonpkgref(); i++ {
				sym := pkg.addSym(r, uint32(i), &goArchive)
				goArchive.entries[goArchive.entryId].syms = append(goArchive.entries[goArchive.entryId].syms, sym)
			}
			for _, importPkg := range r.Autolib() {
				path := importPkg.Pkg
				path = path[:len(path)-len(filepath.Ext(path))]
				pkg.ImportPkgs = append(pkg.ImportPkgs, path)
			}
			goArchive.entryId++
		case archive.EntryNativeObj:
			//CGo files must be parsed by an elf/macho etc. native reader
		default:
			return fmt.Errorf("Parse open %s: unrecognized archive member %s\n", file.Name(), e.Name)
		}
	}
	return nil
}

func (pkg *Pkg) AddCgoFuncs(cgoFuncs map[string]int) {
	goArchive := pkg.GoArchive
	for objIndex := 0; objIndex < goArchive.entryId; objIndex++ {
		r := goArchive.entries[objIndex].r
		// cgo function wrapper
		for i, sym := range goArchive.entries[objIndex].syms {
			if !isRef(r, i) && !isTypeName(sym.Name) && symkind.IsText(sym.Kind) && sym.ABI == uint(obj.ABIInternal) {
				if index, ok := goArchive.symVersions[obj.ABI0][sym.Name]; ok && !isRef(goArchive.entries[index.entryIndex].r, index.symbolIndex) {
					nsym := goArchive.entries[index.entryIndex].syms[index.symbolIndex]
					cgoFuncs[sym.Name] = nsym.Kind
					if (nsym.Func != nil && isWrapperFunctionID(nsym.Func.FuncID)) || isWrapperFunctionID(sym.Func.FuncID) {
						nsym.Name = nsym.Name + ABI0_SUFFIX
					}
				}
			}
		}
	}
}

func (pkg *Pkg) AddSymIndex(cgoFuncs map[string]int) {
	goArchive := pkg.GoArchive
	if goArchive == nil {
		return
	}
	for objIndex := 0; objIndex < goArchive.entryId; objIndex++ {
		// add symbol name in SymIndex. other packages will reference them
		goArchive.entries[objIndex].startIndex = len(pkg.SymIndex)
		for _, sym := range goArchive.entries[objIndex].syms {
			if kind, ok := cgoFuncs[sym.Name]; ok && kind > symkind.Sxxx && kind <= symkind.STLSBSS {
				if sym.ABI == uint(obj.ABI0) {
					sym.Name += ABI0_SUFFIX
				}
			}

			if sym.Kind > symkind.Sxxx && sym.Kind <= symkind.STLSBSS && sym.Name != EmptyString {
				if _, ok := pkg.Syms[sym.Name]; !ok || !sym.DupOK {
					pkg.Syms[sym.Name] = sym
				}
			}
			pkg.SymIndex = append(pkg.SymIndex, sym.Name)
		}
	}
}

func (pkg *Pkg) ResolveSymbols(packages map[string]*Pkg, ObjSymbolMap map[string]*ObjSymbol, CUOffset int32) {
	goArchive := pkg.GoArchive
	if goArchive == nil {
		return
	}

	for entryId := 0; entryId < goArchive.entryId; entryId++ {
		// resolve symbol name
		cache := make(map[string]string, 0)
		for _, sym := range goArchive.entries[entryId].syms {
			sym.Type = resolveSymRefName(sym.Type, packages, pkg.PkgPath, entryId, cache)
			if sym.Func != nil {
				for index, data := range sym.Func.FuncData {
					sym.Func.FuncData[index] = resolveSymRefName(data, packages, pkg.PkgPath, entryId, cache)
				}
				for index, inl := range sym.Func.InlTree {
					sym.Func.InlTree[index].Func = resolveSymRefName(inl.Func, packages, pkg.PkgPath, entryId, cache)
				}
				sym.Func.CUOffset += CUOffset
			}
			for index, reloc := range sym.Reloc {
				sym.Reloc[index].SymName = resolveSymRefName(reloc.SymName, packages, pkg.PkgPath, entryId, cache)
			}
		}
		for _, sym := range pkg.Syms {
			replacePkgPath(sym, pkg.PkgPath)
			ObjSymbolMap[sym.Name] = sym
		}
	}
}

//go:inline
func isRef(r *goobj.Reader, index int) bool {
	return index >= r.NSym()+r.NHashed64def()+r.NHasheddef()+r.NNonpkgdef()
}

func symRef2Index(s goobj.SymRef, r *goobj.Reader) uint32 {
	index := InvalidIndex
	switch p := s.PkgIdx; p {
	case goobj.PkgIdxInvalid:
		if s.SymIdx != 0 {
			panic("bad sym ref")
		}
		return index
	case goobj.PkgIdxHashed64:
		index = s.SymIdx + uint32(r.NSym())
	case goobj.PkgIdxHashed:
		index = s.SymIdx + uint32(r.NSym()+r.NHashed64def())
	case goobj.PkgIdxNone:
		index = s.SymIdx + uint32(r.NSym()+r.NHashed64def()+r.NHasheddef())
	case goobj.PkgIdxBuiltin:
		return index
	case goobj.PkgIdxSelf:
		index = s.SymIdx
	default:
		return index
	}
	return index
}

func (pkg *Pkg) addSym(r *goobj.Reader, index uint32, goArchive *Archive) *ObjSymbol {
	s := r.Sym(index)
	symbol := ObjSymbol{Name: s.Name(r), Kind: int(s.Type()), DupOK: s.Dupok(), Size: (int64)(s.Siz()), ABI: uint(s.ABI()), Func: nil}

	if symbol.Kind == symkind.Sxxx || symbol.Name == EmptyString {
		return &symbol
	}

	if symbol.ABI < uint(obj.ABICount) {
		if _, ok := goArchive.symVersions[symbol.ABI][symbol.Name]; !ok {
			goArchive.symVersions[symbol.ABI][symbol.Name] = symIndex{goArchive.entryId, int(index)}
		}
	}

	if index > uint32(r.NSym()+r.NHashed64def()+r.NHasheddef()+r.NNonpkgdef()) {
		return &symbol
	}

	if symbol.Size > 0 {
		symbol.Data = r.Data(index)
		Grow(&symbol.Data, (int)(symbol.Size))
	} else {
		symbol.Data = make([]byte, 0)
	}

	if symkind.IsText(symbol.Kind) {
		symbol.Func = &FuncInfo{}
	}
	auxs := r.Auxs(index)
	for k := 0; k < len(auxs); k++ {
		index := symRef2Index(auxs[k].Sym(), r)
		switch auxs[k].Type() {
		case goobj.AuxGotype:
			symbol.Type = encodeSymRef(auxs[k].Sym())
		case goobj.AuxFuncInfo:
			funcInfo := goobj.FuncInfo{}
			readFuncInfo(&funcInfo, r.Data(index), symbol.Func)
			symbol.Func.CUOffset = 0
			for _, index := range funcInfo.File {
				symbol.Func.File = append(symbol.Func.File, r.File(int(index)))
			}
			for _, inl := range funcInfo.InlTree {
				inlNode := InlTreeNode{
					Parent:   int64(inl.Parent),
					File:     r.File(int(inl.File)),
					Line:     int64(inl.Line),
					Func:     encodeSymRef(inl.Func),
					ParentPC: int64(inl.ParentPC),
				}
				symbol.Func.InlTree = append(symbol.Func.InlTree, inlNode)
			}
		case goobj.AuxFuncdata:
			symbol.Func.FuncData = append(symbol.Func.FuncData, encodeSymRef(auxs[k].Sym()))
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
	}

	relocs := r.Relocs(index)
	for k := 0; k < len(relocs); k++ {
		symbol.Reloc = append(symbol.Reloc, Reloc{
			Add:     int(relocs[k].Add()),
			Offset:  int(relocs[k].Off()),
			Size:    int(relocs[k].Siz()),
			Type:    int(relocs[k].Type()),
			SymName: encodeSymRef(relocs[k].Sym()),
		})
	}

	return &symbol
}

//go:inline
func isWrapperFunctionID(aFuncId uint8) bool {
	return aFuncId == uint8(objabi.GetFuncID(``, true))
}
