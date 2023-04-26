//go:build go1.16 && !go1.21
// +build go1.16,!go1.21

package obj

import (
	"bytes"
	"cmd/objfile/archive"
	"cmd/objfile/goobj"
	"cmd/objfile/obj"
	"cmd/objfile/objabi"
	"compress/zlib"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/eh-steve/goloader/objabi/reloctype"
	"github.com/eh-steve/goloader/objabi/symkind"
	"go/token"
	"io"
	"sort"
	"strings"
)

func (pkg *Pkg) Symbols() error {
	a, err := archive.Parse(pkg.F, false)
	if err != nil {
		return err
	}

	for _, e := range a.Entries {
		switch e.Type {
		case archive.EntryPkgDef:
			// nothing todo
		case archive.EntryGoObj:
			b := make([]byte, e.Obj.Size)
			_, err := pkg.F.ReadAt(b, e.Obj.Offset)
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
			prevPkgLen := len(pkg.ReferencedPkgs)
			npkg := r.NPkg()
			pkg.ReferencedPkgs = append(pkg.ReferencedPkgs, make([]string, npkg)...)

			for i := 1; i < npkg; i++ { // PkgIdx 0 is a dummy invalid package
				pkgName := r.Pkg(i)
				pkg.ReferencedPkgs[i+prevPkgLen] = pkgName
			}
			pkg.Arch = e.Obj.Arch
			nsym := r.NSym() + r.NHashed64def() + r.NHasheddef() + r.NNonpkgdef()
			for i := 0; i < nsym; i++ {
				pkg.addSym(r, uint32(i), &refNames, objabi.PathToPrefix(pkg.PkgPath))
			}
			files := make([]string, r.NFile())
			for i := range files {
				files[i] = r.File(i)
			}

			pkg.CUFiles = append(pkg.CUFiles, CompilationUnitFiles{
				ArchiveName: e.Name,
				Files:       files,
			})
		case archive.EntryNativeObj:
			// CGo files must be parsed by an elf/macho etc. native reader
			nr := io.NewSectionReader(pkg.F, e.Offset, e.Size)
			elfFile, err := elf.NewFile(nr)
			if err != nil {
				_, _ = nr.Seek(0, 0)
				machoFile, errMacho := macho.NewFile(nr)
				if errMacho != nil {
					return fmt.Errorf("only elf and macho relocations currently supported, failed to open as eitehr: (%s): %w", err, errMacho)
				}
				err = pkg.convertMachoRelocs(machoFile, e)
				if err != nil {
					return err
				}
			} else {
				err = pkg.convertElfRelocs(elfFile, e)
				if err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("Parse open %s: unrecognized archive member %s (%d)\n", pkg.F.Name(), e.Name, e.Type)
		}
	}
	for _, sym := range pkg.Syms {
		if !strings.HasPrefix(sym.Name, TypeStringPrefix) {
			sym.Name = strings.Replace(sym.Name, EmptyPkgPath, pkg.PkgPath, -1)
		}
	}
	for i, symName := range pkg.SymNameOrder {
		if !strings.HasPrefix(symName, TypeStringPrefix) {
			pkg.SymNameOrder[i] = strings.Replace(symName, EmptyPkgPath, pkg.PkgPath, -1)
		}
	}
	return nil
}

func typePkgPath(symName string) (pkgName string) {
	if strings.HasPrefix(symName, TypePrefix) && !strings.HasPrefix(symName, TypeDoubleDotPrefix) {
		typeName := strings.TrimLeft(strings.TrimPrefix(symName, TypePrefix), "*")
		if !strings.HasPrefix(typeName, "func(") &&
			!strings.HasPrefix(typeName, "noalg") &&
			!strings.HasPrefix(typeName, "map[") &&
			!strings.HasPrefix(typeName, "map.bucket[") &&
			!strings.HasPrefix(typeName, "map.iter[") &&
			!strings.HasPrefix(typeName, "map.hdr[") &&
			!strings.HasPrefix(typeName, "struct {") {
			// Likely a named type defined in a package, but for some reason PkgIdx is PkgIdxNone
			if strings.Count(typeName, ".") > 0 {
				pkgName = funcPkgPath(typeName)
			}
		}
	}
	return
}

// TODO - share this implementation with goloader's
func funcPkgPath(funcName string) string {
	funcName = strings.TrimPrefix(funcName, TypeDoubleDotPrefix+"eq.")

	// Anonymous struct methods can't have a package
	if strings.HasPrefix(funcName, "go"+ObjSymbolSeparator+"struct {") || strings.HasPrefix(funcName, "go"+ObjSymbolSeparator+"*struct {") || strings.HasPrefix(funcName, "struct {") {
		return ""
	}
	lastSlash := strings.LastIndexByte(funcName, '/')
	if lastSlash == -1 {
		lastSlash = 0
	}

	// Generic dictionaries
	firstDict := strings.Index(funcName, "..dict")
	if firstDict > 0 {
		return funcName[:firstDict]
	} else {
		// Methods on structs embedding structs from other packages look funny, e.g.:
		// regexp.(*onePassInst).regexp/syntax.op
		firstBracket := strings.LastIndex(funcName, ".(")
		if firstBracket > 0 && lastSlash > firstBracket {
			lastSlash = firstBracket
		}
		firstSquareBracket := strings.Index(funcName, "[")
		if firstSquareBracket > 0 && lastSlash > firstSquareBracket {
			i := firstSquareBracket
			for ; funcName[i] != '.' && i > 0; i-- {
			}
			return funcName[:i]
		}
	}

	dot := lastSlash
	for ; dot < len(funcName) && funcName[dot] != '.' && funcName[dot] != '(' && funcName[dot] != '['; dot++ {
	}
	pkgPath := funcName[:dot]
	return strings.Trim(strings.TrimPrefix(pkgPath, "[...]"), " ")
}

func resolveSymRef(s goobj.SymRef, r *goobj.Reader, refNames *map[goobj.SymRef]string, pkgName string) (string, string, uint32) {
	i := InvalidIndex
	switch p := s.PkgIdx; p {
	case goobj.PkgIdxInvalid:
		if s.SymIdx != 0 {
			panic("bad sym ref")
		}
		return EmptyString, "", i
	case goobj.PkgIdxHashed64:
		i = s.SymIdx + uint32(r.NSym())
	case goobj.PkgIdxHashed:
		i = s.SymIdx + uint32(r.NSym()+r.NHashed64def())
	case goobj.PkgIdxNone:
		i = s.SymIdx + uint32(r.NSym()+r.NHashed64def()+r.NHasheddef())
		symName := r.Sym(i).Name(r)
		if (strings.HasPrefix(symName, TypePrefix) && !strings.HasPrefix(symName, TypeDoubleDotPrefix+"eq.")) || strings.HasPrefix(symName, "go"+ObjSymbolSeparator+"info") || strings.HasPrefix(symName, "go"+ObjSymbolSeparator+"cuinfo") || strings.HasPrefix(symName, "go"+ObjSymbolSeparator+"interface {") {
			pkgName = typePkgPath(symName)
		} else {
			pkgName = funcPkgPath(symName)
		}
	case goobj.PkgIdxBuiltin:
		name, _ := goobj.BuiltinName(int(s.SymIdx))
		return name, "", i
	case goobj.PkgIdxSelf:
		i = s.SymIdx
	default:
		return (*refNames)[s], r.Pkg(int(s.PkgIdx)), i
	}
	return r.Sym(i).Name(r), pkgName, i
}

func UnresolvedIdxString(symRef goobj.SymRef) string {
	buf := make([]byte, len(UnresolvedSymRefPrefix)+8+8)
	copy(buf, UnresolvedSymRefPrefix)
	uint32buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(uint32buf, symRef.PkgIdx)
	hex.Encode(buf[len(UnresolvedSymRefPrefix):], uint32buf)
	binary.LittleEndian.PutUint32(uint32buf, symRef.SymIdx)
	hex.Encode(buf[len(UnresolvedSymRefPrefix)+8:], uint32buf)
	return string(buf)
}

func ParseUnresolvedIdxString(unresolved string) goobj.SymRef {
	buf := make([]byte, 8)
	n, err := hex.Decode(buf, []byte(unresolved[len(UnresolvedSymRefPrefix):]))
	if err != nil || n != 8 {
		panic(fmt.Sprintf("failed to decode %s: %s", unresolved, err))
	}
	return goobj.SymRef{
		PkgIdx: binary.LittleEndian.Uint32(buf),
		SymIdx: binary.LittleEndian.Uint32(buf[4:]),
	}
}

func (pkg *Pkg) addSym(r *goobj.Reader, idx uint32, refNames *map[goobj.SymRef]string, pkgPath string) {
	s := r.Sym(idx)
	symbol := ObjSymbol{Name: s.Name(r), Kind: int(s.Type()), DupOK: s.Dupok(), Size: (int64)(s.Siz()), Func: &FuncInfo{ABI: s.ABI()}, Objidx: pkg.Objidx, Pkg: pkgPath}
	if original, ok := pkg.Syms[symbol.Name]; ok {
		if objabi.SymKind(symbol.Kind) == objabi.STEXT && obj.ABI(symbol.Func.ABI) == obj.ABI0 {
			// Valid duplicate symbol names may be caused by ASM ABI0 versions of functions, and their autogenerated ABIInternal wrappers
			// We can keep the wrapper func under a separate mangled symbol name in case it's needed for direct calls,
			// and replace the main symbol with the ABI0 version (the compiler will have inlined the wrapper at any callsites anyway)
			// https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md
			if obj.ABI(original.Func.ABI) == obj.ABIInternal && !strings.HasPrefix(original.Name, "reflect.") { // reflect functions are special wrappers
				original.Name += ABIInternalSuffix
				original.Pkg = symbol.Pkg
				if _, ok := pkg.Syms[original.Name]; !ok {
					pkg.SymNameOrder = append(pkg.SymNameOrder, original.Name)
				}
				pkg.Syms[original.Name] = original
			}
		} else {
			return
		}
	}
	if objabi.SymKind(symbol.Kind) == objabi.Sxxx || symbol.Name == EmptyString {
		return
	}

	if int(idx) > r.NSym()+r.NHashed64def()+r.NHasheddef() {
		// Is nonpkgdef or nonpkgref
		if !strings.HasPrefix(symbol.Name, "_cgo") {
			// TODO - this really isn't ideal - needs to be more robust
			if (strings.HasPrefix(symbol.Name, TypePrefix) && !strings.HasPrefix(symbol.Name, TypeDoubleDotPrefix+"eq.")) || strings.HasPrefix(symbol.Name, "go"+ObjSymbolSeparator+"info") || strings.HasPrefix(symbol.Name, "go"+ObjSymbolSeparator+"cuinfo") || strings.HasPrefix(symbol.Name, "go"+ObjSymbolSeparator+"interface {") {
				symbol.Pkg = ""
			} else {
				symbol.Pkg = funcPkgPath(symbol.Name)
			}
		}
	}
	if objabi.SymKind(symbol.Kind) == objabi.SNOPTRBSS && strings.HasPrefix(symbol.Name, "_cgo_") && symbol.Size == 1 {
		// This is a dummy symbol representing a byte whose address is taken to act as the function pointer to a CGo text address via the //go:linkname pragma
		// We handle this separately at the end of convertMachoRelocs() by adding the actual target address as text under this symbol name.
		return
	}
	if symbol.Size > 0 {
		symbol.Data = r.Data(idx)
		grow(&symbol.Data, (int)(symbol.Size))
	} else {
		symbol.Data = make([]byte, 0)
	}

	if _, ok := pkg.Syms[symbol.Name]; !ok {
		pkg.SymNameOrder = append(pkg.SymNameOrder, symbol.Name)
	}
	pkg.SymNamesByIdx[idx] = symbol.Name
	pkg.Syms[symbol.Name] = &symbol

	auxs := r.Auxs(idx)
	for k := 0; k < len(auxs); k++ {
		auxSymRef := auxs[k].Sym()
		parentPkgPath := pkgPath
		name, pkgPath, index := resolveSymRef(auxSymRef, r, refNames, pkgPath)
		if name == "" || index == InvalidIndex {
			pkg.UnresolvedSymRefs[auxSymRef] = struct{}{}
		}
		switch auxs[k].Type() {
		case goobj.AuxGotype:
			if name == "" {
				// Likely this type is defined in another package not yet loaded, so mark it as unresolved and resolve it later, after all packages
				symbol.Type = UnresolvedIdxString(auxSymRef)
			} else if objabi.SymKind(r.Sym(index).Type()) == objabi.Sxxx {
				// This aux symref doesn't actually exist in the current package reader, so we add a fake reloc to force the package containing the symbol to be built
				symbol.Reloc = append(symbol.Reloc, Reloc{
					Offset: InvalidOffset,
					Sym:    &Sym{Name: name, Offset: InvalidOffset, Pkg: pkgPath},
					Type:   reloctype.R_KEEP,
				})
				symbol.Type = name
			} else {
				symbol.Type = name
				symName := strings.TrimPrefix(symbol.Name, parentPkgPath+".")
				if token.IsExported(symName) {
					pkg.Exports[symName] = ExportSymType{
						SymName:  symbol.Name,
						TypeName: name,
					}
				}
			}
		case goobj.AuxFuncInfo:
			funcInfo := goobj.FuncInfo{}
			readFuncInfo(&funcInfo, r.Data(index), symbol.Func)
			for _, index := range funcInfo.File {
				symbol.Func.File = append(symbol.Func.File, r.File(int(index)))
			}
			cuOffset := 0
			for _, cuFiles := range pkg.CUFiles {
				cuOffset += len(cuFiles.Files)
			}
			symbol.Func.CUOffset = cuOffset
			for _, inl := range funcInfo.InlTree {
				funcname, pkgPath, _ := resolveSymRef(inl.Func, r, refNames, pkgPath)
				funcname = strings.Replace(funcname, EmptyPkgPath, pkgPath, -1)
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
			pkg.addSym(r, index, refNames, pkgPath)
		}
	}

	relocs := r.Relocs(idx)
	priorRelocs := len(symbol.Reloc)
	for k := priorRelocs; k < len(relocs)+priorRelocs; k++ {
		symbol.Reloc = append(symbol.Reloc, Reloc{})
		symbol.Reloc[k].Add = int(relocs[k-priorRelocs].Add())
		symbol.Reloc[k].Offset = int(relocs[k-priorRelocs].Off())
		symbol.Reloc[k].Size = int(relocs[k-priorRelocs].Siz())
		symbol.Reloc[k].Type = int(relocs[k-priorRelocs].Type())
		name, pkgPath, index := resolveSymRef(relocs[k-priorRelocs].Sym(), r, refNames, pkgPath)
		symbol.Reloc[k].Sym = &Sym{Name: name, Offset: InvalidOffset, Pkg: pkgPath}
		if _, ok := pkg.Syms[name]; !ok && index != InvalidIndex {
			pkg.addSym(r, index, refNames, pkgPath)
		}
	}
}

func (pkg *Pkg) convertElfRelocs(f *elf.File, e archive.Entry) error {
	if f.Class != elf.ELFCLASS64 {
		return fmt.Errorf("only 64-bit elf relocations currently supported")
	}
	if f.Machine != elf.EM_X86_64 && f.Machine != elf.EM_AARCH64 {
		return fmt.Errorf("only amd64 and arm64 elf relocations currently supported")
	}

	elfSyms, err := f.Symbols()

	if err != nil {
		return fmt.Errorf("failed to extract symbols from objfile entry %s: %w", e.Name, err)
	}

	var textSect *elf.Section
	var textIndex int
	for i, s := range f.Sections {
		if s.Name == ".text" {
			textSect = s
			textIndex = i
			break
		}
	}

	if textSect == nil {
		return fmt.Errorf("failed to find .text elf section in objfile entry %s: %w", e.Name, err)
	}

	text, err := textSect.Data()
	if err != nil {
		return fmt.Errorf("failed to read text data from elf .text section %s: %w", e.Name, err)
	}
	textOffset := textSect.Addr

	var (
		dlen              uint64
		compressionOffset int
		dbuf              []byte
	)
	if len(text) >= 12 && string(text[:4]) == "ZLIB" {
		dlen = binary.BigEndian.Uint64(text[4:12])
		compressionOffset = 12
	}
	if dlen == 0 && len(text) >= 12 && textSect.Flags&elf.SHF_COMPRESSED != 0 &&
		textSect.Flags&elf.SHF_ALLOC == 0 &&
		f.FileHeader.ByteOrder.Uint32(text[:]) == uint32(elf.COMPRESS_ZLIB) {
		switch f.FileHeader.Class {
		case elf.ELFCLASS32:
			// Chdr32.Size offset
			dlen = uint64(f.FileHeader.ByteOrder.Uint32(text[4:]))
			compressionOffset = 12
		case elf.ELFCLASS64:
			if len(text) < 24 {
				return fmt.Errorf("invalid compress header 64")
			}
			// Chdr64.Size offset
			dlen = f.FileHeader.ByteOrder.Uint64(text[8:])
			compressionOffset = 24
		default:
			return fmt.Errorf("unsupported compress header:%s", f.FileHeader.Class)
		}
	}
	if dlen > 0 {
		dbuf = make([]byte, dlen)
		r, err := zlib.NewReader(bytes.NewBuffer(text[compressionOffset:]))
		if err != nil {
			return fmt.Errorf("failed to decompress zlib elf section %s: %w", e.Name, err)
		}
		if _, err := io.ReadFull(r, dbuf); err != nil {
			return fmt.Errorf("failed to read decompressed zlib elf section %s: %w", e.Name, err)
		}
		if err := r.Close(); err != nil {
			return fmt.Errorf("failed to close zlib elf section %s: %w", e.Name, err)
		}
		text = dbuf
	}

	var objSymbols []*ObjSymbol
	var objSymAddr []uint64
	for _, s := range elfSyms {
		var sym *ObjSymbol
		var addr uint64
		if s.Name != "" && s.Size != 0 {
			addr = s.Value
			data := make([]byte, s.Size)
			copy(data, text[addr+textOffset:])
			sym = &ObjSymbol{Name: s.Name, Data: data, Size: int64(s.Size), Func: &FuncInfo{}, Pkg: pkg.PkgPath}
		}
		objSymbols = append(objSymbols, sym)
		objSymAddr = append(objSymAddr, addr)
		if sym == nil {
			continue
		}

		switch s.Section {
		case elf.SHN_UNDEF:
			sym.Kind = symkind.Sxxx
		case elf.SHN_COMMON:
			sym.Kind = symkind.SBSS
		default:
			i := int(s.Section)
			if i < 0 || i >= len(f.Sections) {
				break
			}
			sect := f.Sections[i]
			switch sect.Flags & (elf.SHF_WRITE | elf.SHF_ALLOC | elf.SHF_EXECINSTR) {
			case elf.SHF_ALLOC | elf.SHF_EXECINSTR:
				sym.Kind = symkind.STEXT
			case elf.SHF_ALLOC:
				sym.Kind = symkind.SRODATA

			case elf.SHF_ALLOC | elf.SHF_WRITE:
				sym.Kind = symkind.SDATA
			}
		}
	}

	for _, r := range f.Sections {
		if r.Type != elf.SHT_RELA && r.Type != elf.SHT_REL {
			continue
		}
		if int(r.Info) != textIndex {
			continue
		}
		rd, err := r.Data()
		if err != nil {
			return fmt.Errorf("failed to read relocation data from elf section %s %s: %w", e.Name, r.Name, err)
		}

		relR := bytes.NewReader(rd)
		var rela elf.Rela64

		for relR.Len() > 0 {
			binary.Read(relR, f.ByteOrder, &rela)
			symNo := rela.Info >> 32
			if symNo == 0 || symNo > uint64(len(elfSyms)) {
				continue
			}
			sym := &elfSyms[symNo-1]

			var target *ObjSymbol
			var targetAddr uint64
			for i, objSymbol := range objSymbols {
				if objSymbol == nil {
					continue
				}
				nextAddr := objSymAddr[i] + uint64(objSymbol.Size)
				if rela.Off >= objSymAddr[i] && rela.Off < nextAddr {
					target = objSymbol
					targetAddr = objSymAddr[i]
					break
				}
			}
			if target == nil {
				fmt.Println("Couldn't find target for offset ", rela.Off, sym.Name)
				continue
			}

			if sym.Section == elf.SHN_UNDEF || sym.Section < elf.SHN_LORESERVE {
				switch f.Machine {
				case elf.EM_AARCH64:
					t := elf.R_AARCH64(rela.Info & 0xffff)
					switch t {
					case elf.R_AARCH64_CALL26, elf.R_AARCH64_JUMP26:
						target.Reloc = append(target.Reloc, Reloc{
							Offset: int(rela.Off - targetAddr),
							Sym:    &Sym{Name: sym.Name, Offset: InvalidOffset},
							Size:   4,
							Type:   reloctype.R_CALLARM64,
							Add:    0, // Even though elf addend is -4, a Go PCREL reloc doesn't need this.
						})
					default:
						return fmt.Errorf("only a limited subset of elf relocations currently supported, got %s for symbol %s reloc to %s", t.GoString(), target.Name, sym.Name)
					}
				case elf.EM_X86_64:
					t := elf.R_X86_64(rela.Info & 0xffff)
					switch t {
					case elf.R_X86_64_64, elf.R_X86_64_32:
						return fmt.Errorf("TODO: only a limited subset of elf relocations currently supported, got %s", t.GoString())
					case elf.R_X86_64_PLT32, elf.R_X86_64_PC32:
						target.Reloc = append(target.Reloc, Reloc{
							Offset: int(rela.Off - targetAddr),
							Sym:    &Sym{Name: sym.Name, Offset: InvalidOffset},
							Size:   4,
							Type:   reloctype.R_PCREL,
							Add:    0, // Even though elf addend is -4, a Go PCREL reloc doesn't need this.
						})
					default:
						return fmt.Errorf("only a limited subset of elf relocations currently supported, got %s for symbol %s reloc to %s", t.GoString(), target.Name, sym.Name)
					}
				}
			} else {
				return fmt.Errorf("got an unexpected symbol section %d", sym.Section)
			}
		}
	}

	for _, symbol := range objSymbols {
		if symbol != nil && symbol.Name != "" && symbol.Size > 0 && symbol.Kind == symkind.STEXT {
			if _, ok := pkg.Syms[symbol.Name]; !ok {
				pkg.SymNameOrder = append(pkg.SymNameOrder, symbol.Name)
			}
			pkg.Syms[symbol.Name] = symbol
		}
	}
	return nil
}

type uint64s []uint64

func (x uint64s) Len() int           { return len(x) }
func (x uint64s) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }
func (x uint64s) Less(i, j int) bool { return x[i] < x[j] }

func (pkg *Pkg) convertMachoRelocs(f *macho.File, e archive.Entry) error {
	if f.Symtab == nil {
		return nil
	}
	var text []byte
	var err error
	var textSect = f.Section("__text")

	if textSect == nil {
		return nil
	}
	text, err = textSect.Data()
	if err != nil {
		return fmt.Errorf("failed to read __text section data from %s: %w", e.Name, err)
	}

	// Build sorted list of addresses of all symbols.
	// We infer the size of a symbol by looking at where the next symbol begins.
	var addrs []uint64
	for _, s := range f.Symtab.Syms {
		// Skip stab debug info.
		if s.Type&0xe0 == 0 {
			addrs = append(addrs, s.Value)
		}
	}
	sort.Sort(uint64s(addrs))

	var objSymbols []*ObjSymbol
	for _, s := range f.Symtab.Syms {
		if s.Type&0xe0 != 0 {
			// Skip stab debug info.
			continue
		}

		if s.Name == "" || s.Sect == 0 {
			continue
		}

		var sym *ObjSymbol
		var addr uint64

		sym = &ObjSymbol{Name: s.Name, Func: &FuncInfo{}, Pkg: pkg.PkgPath}

		i := sort.Search(len(addrs), func(x int) bool { return addrs[x] > s.Value })
		if i < len(addrs) {
			sym.Size = int64(addrs[i] - s.Value)
		} else {
			sym.Size = int64(len(text))
		}

		if sym.Size > 0 && s.Sect > 0 && f.Sections[s.Sect-1] == textSect {
			addr = s.Value
			data := make([]byte, sym.Size)
			copy(data, text[addr:])
			sym.Data = data
		}

		objSymbols = append(objSymbols, sym)

		if int(s.Sect) <= len(f.Sections) {
			sect := f.Sections[s.Sect-1]
			switch sect.Seg {
			case "__TEXT", "__DATA_CONST":
				sym.Kind = symkind.SRODATA
			case "__DATA":
				sym.Kind = symkind.SDATA
			}
			switch sect.Seg + " " + sect.Name {
			case "__TEXT __text":
				sym.Kind = symkind.STEXT
			case "__DATA __bss":
				sym.Kind = symkind.SBSS
			case "__DATA __noptrbss":
				sym.Kind = symkind.SNOPTRBSS
			}
		}

		for _, reloc := range append(textSect.Relocs) {
			if uint64(reloc.Addr) < s.Value+uint64(sym.Size) && uint64(reloc.Addr) > s.Value {
				// when Scattered == false && Extern == true, Value is the symbol number.
				// when Scattered == false && Extern == false, Value is the section number.
				// when Scattered == true, Value is the value that this reloc refers to.

				if pkg.Arch == "arm64" {
					if !reloc.Scattered && reloc.Extern && reloc.Pcrel {
						sym.Reloc = append(sym.Reloc, Reloc{
							Offset: int(uint64(reloc.Addr) - s.Value),
							Sym:    &Sym{Name: f.Symtab.Syms[reloc.Value].Name, Offset: InvalidOffset},
							Size:   4,
							Type:   reloctype.R_CALLARM64,
							Add:    0,
						})
					} else {
						return fmt.Errorf("got an unsupported macho reloc: %#v", reloc)
					}
				} else if pkg.Arch == "amd64" {
					if !reloc.Scattered && reloc.Extern && reloc.Pcrel {
						sym.Reloc = append(sym.Reloc, Reloc{
							Offset: int(uint64(reloc.Addr) - s.Value),
							Sym:    &Sym{Name: f.Symtab.Syms[reloc.Value].Name, Offset: InvalidOffset},
							Size:   4,
							Type:   reloctype.R_PCREL,
							Add:    0,
						})
					} else {
						return fmt.Errorf("got an unsupported macho reloc: %#v", reloc)
					}
				} else {
					return fmt.Errorf("unsupported arch: %s", pkg.Arch)
				}

			}
		}
	}

	for _, symbol := range objSymbols {
		if _, ok := pkg.Syms[symbol.Name]; !ok {
			pkg.SymNameOrder = append(pkg.SymNameOrder, symbol.Name)
		}
		pkg.Syms[symbol.Name] = symbol
		if strings.HasPrefix(symbol.Name, "__cgo_") {
			// Need to add symbol as _cgo_* so that the Go generated
			pkg.Syms[symbol.Name[1:]] = &ObjSymbol{
				Name:  symbol.Name[1:],
				Kind:  symbol.Kind,
				DupOK: true,
				Size:  symbol.Size,
				Data:  symbol.Data,
				Type:  symbol.Type,
				Reloc: symbol.Reloc,
				Func:  symbol.Func,
				Pkg:   symbol.Pkg,
			}
		}
	}
	return nil
}
