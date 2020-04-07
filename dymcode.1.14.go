// +build go1.14
// +build !go1.15

package goloader

const (
	R_PCREL = 16
	// R_TLS_LE, used on 386, amd64, and ARM, resolves to the offset of the
	// thread-local symbol from the thread local base and is used to implement the
	// "local exec" model for tls access (r.Sym is not set on intel platforms but is
	// set to a TLS symbol -- runtime.tlsg -- in the linker when externally linking).
	R_TLS_LE = 17
	// R_METHODOFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	// It is a variant of R_ADDROFF used when linking from the uncommonType of a
	// *rtype, and may be set to zero by the linker if it determines the method
	// text is unreachable by the linked program.
	R_METHODOFF = 25
)

func addStackObject(code *CodeReloc, fi *funcInfoData, seg *segment, symPtr map[string]uintptr) {
	_addStackObject(code, fi, seg, symPtr)
}

func addDeferReturn(code *CodeReloc, fi *funcInfoData, seg *segment) {
	if len(fi.funcdata) > _FUNCDATA_OpenCodedDeferInfo && fi.funcdata[_FUNCDATA_OpenCodedDeferInfo] != 0xFFFFFFFF {
		sym := code.Syms[code.SymMap[fi.name]]
		for _, r := range sym.Reloc {
			if r.SymOff == code.SymMap["runtime.deferreturn"] {
				//../cmd/link/internal/ld/pcln.go:pclntab
				switch code.Arch {
				case "amd64", "386":
					fi.deferreturn = uint32(r.Offset) - uint32(sym.Offset) - 1
				case "arm", "arm64":
					fi.deferreturn = uint32(r.Offset) - uint32(sym.Offset)
				default:
					sprintf(&seg.err, "not support arch:", code.Arch, "\n")
				}
				break
			}
		}
	}
}
