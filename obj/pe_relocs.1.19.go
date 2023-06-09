//go:build go1.19
// +build go1.19

package obj

import (
	"cmd/objfile/archive"
	"cmd/objfile/objabi"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"github.com/eh-steve/goloader/objabi/symkind"
	"sort"
	"strings"
)

// See cmd/link/internal/loadpd/ldpe.go for reference
const (
	IMAGE_SYM_CLASS_STATIC   = 3
	IMAGE_REL_I386_DIR32     = 0x0006
	IMAGE_REL_I386_DIR32NB   = 0x0007
	IMAGE_REL_I386_REL32     = 0x0014
	IMAGE_REL_AMD64_ADDR64   = 0x0001
	IMAGE_REL_AMD64_ADDR32   = 0x0002
	IMAGE_REL_AMD64_ADDR32NB = 0x0003
	IMAGE_REL_AMD64_REL32    = 0x0004
)

func readPESym(arch string, pesym *pe.COFFSymbol, f *pe.File, defWithImp map[string]struct{}) (string, error) {
	symname, err := pesym.FullName(f.StringTable)
	if err != nil {
		return "", fmt.Errorf("failed to read full name of pesym: %w", err)
	}
	symIsSect := pesym.StorageClass == IMAGE_SYM_CLASS_STATIC && pesym.Type == 0 && pesym.Name[0] == '.'
	var name string
	if symIsSect {
		name = f.Sections[pesym.SectionNumber-1].Name
	} else {
		name = symname
		if strings.HasPrefix(symname, "__imp_") {
			orig := symname[len("__imp_"):]
			if _, ok := defWithImp[orig]; ok {
				// Don't rename __imp_XXX to XXX, since if we do this
				// we'll wind up with a duplicate definition. One
				// example is "__acrt_iob_func"; see commit b295099
				// from git://git.code.sf.net/p/mingw-w64/mingw-w64
				// for details.
			} else {
				name = strings.TrimPrefix(name, "__imp_") // __imp_Name => Name
			}
		}
		// A note on the "_main" exclusion below: the main routine
		// defined by the Go runtime is named "_main", not "main", so
		// when reading references to _main from a host object we want
		// to avoid rewriting "_main" to "main" in this specific
		// instance. See #issuecomment-1143698749 on #35006 for more
		// details on this problem.

		if arch == "386" && name[0] == '_' && name != "_main" {
			name = name[1:] // _Name => Name
		}
	}

	// remove last @XXX
	if i := strings.LastIndex(name, "@"); i >= 0 {
		name = name[:i]
	}
	return name, nil
}

func preprocessSymbols(f *pe.File) (map[string]struct{}, error) {

	// preprocessSymbols walks the COFF symbols for the PE file we're
	// reading and looks for cases where we have both a symbol definition
	// for "XXX" and an "__imp_XXX" symbol, recording these cases in a map
	// in the state struct. This information will be used in readpesym()
	// above to give such symbols special treatment. This function also
	// gathers information about COMDAT sections/symbols for later use
	// in readpesym().

	// Locate comdat sections.
	comdats := make(map[uint16]int64)
	for i, s := range f.Sections {
		if s.Characteristics&uint32(pe.IMAGE_SCN_LNK_COMDAT) != 0 {
			comdats[uint16(i)] = int64(s.Size)
		}
	}

	// Examine symbol defs.
	imp := make(map[string]struct{})
	def := make(map[string]struct{})
	for i, numaux := 0, 0; i < len(f.COFFSymbols); i += numaux + 1 {
		pesym := &f.COFFSymbols[i]
		numaux = int(pesym.NumberOfAuxSymbols)
		if pesym.SectionNumber == 0 { // extern
			continue
		}
		symname, err := pesym.FullName(f.StringTable)
		if err != nil {
			return nil, err
		}
		def[symname] = struct{}{}
		if strings.HasPrefix(symname, "__imp_") {
			imp[strings.TrimPrefix(symname, "__imp_")] = struct{}{}
		}
		if _, isc := comdats[uint16(pesym.SectionNumber-1)]; !isc {
			continue
		}
		if pesym.StorageClass != uint8(IMAGE_SYM_CLASS_STATIC) {
			continue
		}
		// This symbol corresponds to a COMDAT section. Read the
		// aux data for it.
		auxsymp, err := f.COFFSymbolReadSectionDefAux(i)
		if err != nil {
			return nil, fmt.Errorf("unable to read aux info for section def symbol %d %s: pe.COFFSymbolReadComdatInfo returns %v", i, symname, err)
		}
		if auxsymp.Selection == pe.IMAGE_COMDAT_SELECT_SAME_SIZE {
			// This is supported.
		} else if auxsymp.Selection == pe.IMAGE_COMDAT_SELECT_ANY {
			// Also supported.
			comdats[uint16(pesym.SectionNumber-1)] = int64(-1)
		} else {
			// We don't support any of the other strategies at the
			// moment. I suspect that we may need to also support
			// "associative", we'll see.
			return nil, fmt.Errorf("internal error: unsupported COMDAT selection strategy found in sec=%d strategy=%d idx=%d, please file a bug", auxsymp.SecNum, auxsymp.Selection, i)
		}
	}
	defWithImp := make(map[string]struct{})
	for n := range imp {
		if _, ok := def[n]; ok {
			defWithImp[n] = struct{}{}
		}
	}
	return defWithImp, nil
}

func issectCoff(s *pe.COFFSymbol) bool {
	return s.StorageClass == IMAGE_SYM_CLASS_STATIC && s.Type == 0 && s.Name[0] == '.'
}

func issect(s *pe.Symbol) bool {
	return s.StorageClass == IMAGE_SYM_CLASS_STATIC && s.Type == 0 && s.Name[0] == '.'
}

func (pkg *Pkg) convertPERelocs(f *pe.File, e archive.Entry) error {
	textSect := f.Section(".text")
	if textSect == nil {
		return nil
	}
	defWithImp, err := preprocessSymbols(f)
	if err != nil {
		return err
	}

	// Build sorted list of addresses of all symbols.
	// We infer the size of a symbol by looking at where the next symbol begins.
	var objSymAddr []uint32
	for _, s := range f.Symbols {
		if issect(s) || s.SectionNumber <= 0 || int(s.SectionNumber) > len(f.Sections) {
			continue
		}
		objSymAddr = append(objSymAddr, s.Value)
	}

	var objSymbols []*ObjSymbol

	for _, s := range f.Symbols {
		if issect(s) || s.SectionNumber <= 0 || int(s.SectionNumber) > len(f.Sections) {
			continue
		}

		sect := f.Sections[s.SectionNumber-1]
		text, err := sect.Data()
		if err != nil {
			return fmt.Errorf("failed to read section data for symbol %s, section %d: %w", s.Name, s.SectionNumber, err)
		}

		var sym *ObjSymbol
		var addr uint32

		sym = &ObjSymbol{Name: s.Name, Func: &FuncInfo{}, Pkg: pkg.PkgPath}

		i := sort.Search(len(objSymAddr), func(x int) bool { return objSymAddr[x] > s.Value })
		if i < len(objSymAddr) {
			sym.Size = int64(objSymAddr[i] - s.Value)
		} else {
			sym.Size = int64(len(text))
		}

		if sym.Size > 0 && s.SectionNumber > 0 && f.Sections[s.SectionNumber-1] == textSect {
			addr = s.Value
			data := make([]byte, sym.Size)
			copy(data, text[addr:])
			sym.Data = data
		}

		objSymbols = append(objSymbols, sym)

		switch sect.Characteristics & (pe.IMAGE_SCN_CNT_UNINITIALIZED_DATA | pe.IMAGE_SCN_CNT_INITIALIZED_DATA | pe.IMAGE_SCN_MEM_READ | pe.IMAGE_SCN_MEM_WRITE | pe.IMAGE_SCN_CNT_CODE | pe.IMAGE_SCN_MEM_EXECUTE) {
		case pe.IMAGE_SCN_CNT_INITIALIZED_DATA | pe.IMAGE_SCN_MEM_READ: //.rdata
			sym.Kind = symkind.SRODATA

		case pe.IMAGE_SCN_CNT_UNINITIALIZED_DATA | pe.IMAGE_SCN_MEM_READ | pe.IMAGE_SCN_MEM_WRITE: //.bss
			sym.Kind = symkind.SNOPTRBSS

		case pe.IMAGE_SCN_CNT_INITIALIZED_DATA | pe.IMAGE_SCN_MEM_READ | pe.IMAGE_SCN_MEM_WRITE: //.data
			sym.Kind = symkind.SNOPTRDATA

		case pe.IMAGE_SCN_CNT_CODE | pe.IMAGE_SCN_MEM_EXECUTE | pe.IMAGE_SCN_MEM_READ: //.text
			sym.Kind = symkind.STEXT
		default:
			return fmt.Errorf("unexpected flags %#06x for PE section %s", sect.Characteristics, sect.Name)

		}
	}

	for _, section := range f.Sections {
		if section != textSect {
			// Only relocate inside .text
			// TODO - does this make sense?
			continue
		}
		if section.NumberOfRelocations == 0 {
			continue
		}
		if section.Characteristics&pe.IMAGE_SCN_MEM_DISCARDABLE != 0 {
			continue
		}
		if section.Characteristics&(pe.IMAGE_SCN_CNT_CODE|pe.IMAGE_SCN_CNT_INITIALIZED_DATA|pe.IMAGE_SCN_CNT_UNINITIALIZED_DATA) == 0 {
			// This has been seen for .idata sections, which we
			// want to ignore. See issues 5106 and 5273.
			continue
		}

		sectdata, err := section.Data()
		if err != nil {
			return fmt.Errorf("failed to read section data for %s: %w", section.Name, err)
		}
		for j, reloc := range section.Relocs {
			if int(reloc.SymbolTableIndex) >= len(f.COFFSymbols) {
				return fmt.Errorf("relocation number %d symbol index idx=%d cannot be larger than number of symbols %d", j, reloc.SymbolTableIndex, len(f.COFFSymbols))
			}
			pesym := f.COFFSymbols[reloc.SymbolTableIndex]
			relocSymName, err := readPESym(pkg.Arch, &pesym, f, defWithImp)
			if err != nil {
				return err
			}
			if section.Name == relocSymName {
				continue
			}
			rSize := uint8(4)
			var rAdd int64
			var rType objabi.RelocType
			rOff := int32(reloc.VirtualAddress)

			switch reloc.Type {
			case IMAGE_REL_I386_REL32, IMAGE_REL_AMD64_REL32,
				IMAGE_REL_AMD64_ADDR32, // R_X86_64_PC32
				IMAGE_REL_AMD64_ADDR32NB:
				rType = objabi.R_PCREL
				rAdd = int64(int32(binary.LittleEndian.Uint32(sectdata[rOff:])))

			case IMAGE_REL_I386_DIR32NB, IMAGE_REL_I386_DIR32:
				rType = objabi.R_ADDR

				// load addend from image
				rAdd = int64(int32(binary.LittleEndian.Uint32(sectdata[rOff:])))

			case IMAGE_REL_AMD64_ADDR64: // R_X86_64_64
				rSize = 8

				rType = objabi.R_ADDR

				// load addend from image
				rAdd = int64(binary.LittleEndian.Uint64(sectdata[rOff:]))
			default:
				return fmt.Errorf("unsupported PE relocation type: %d in symbol %s", reloc.Type, relocSymName)
			}

			var target *ObjSymbol
			var targetAddr uint32
			for i, objSymbol := range objSymbols {
				if objSymbol == nil {
					continue
				}
				nextAddr := objSymAddr[i] + uint32(objSymbol.Size)
				if reloc.VirtualAddress >= objSymAddr[i] && reloc.VirtualAddress < nextAddr {
					target = objSymbol
					targetAddr = objSymAddr[i]
					break
				}
			}
			if target == nil {
				fmt.Println("Goloader PE Reloc error - couldn't find target for offset ", reloc.VirtualAddress, relocSymName)
				continue
			}

			if issectCoff(&pesym) {
				rAdd += int64(pesym.Value)
			}

			target.Reloc = append(target.Reloc, Reloc{
				Offset: int(reloc.VirtualAddress - targetAddr),
				Sym:    &Sym{Name: relocSymName, Offset: InvalidOffset},
				Size:   int(rSize),
				Type:   int(rType),
				Add:    int(rAdd),
			})
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
