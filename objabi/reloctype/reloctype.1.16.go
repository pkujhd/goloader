//go:build go1.16 && !go1.17
// +build go1.16,!go1.17

package reloctype

import "cmd/objfile/objabi"

// copy from $GOROOT/src/cmd/internal/objabi/reloctype.go
const (
	R_ADDR = (int)(objabi.R_ADDR)
	// R_ADDRARM64 relocates an adrp, add pair to compute the address of the
	// referenced symbol.
	R_ADDRARM64 = (int)(objabi.R_ADDRARM64)
	// R_ADDROFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	R_ADDROFF = (int)(objabi.R_ADDROFF)
	// R_WEAKADDROFF resolves just like R_ADDROFF but is a weak relocation.
	// A weak relocation does not make the symbol it refers to reachable,
	// and is only honored by the linker if the symbol is in some other way
	// reachable.
	R_WEAKADDROFF = (int)(objabi.R_WEAKADDROFF)
	R_CALL        = (int)(objabi.R_CALL)
	R_CALLARM     = (int)(objabi.R_CALLARM)
	R_CALLARM64   = (int)(objabi.R_CALLARM64)
	R_CALLIND     = (int)(objabi.R_CALLIND)
	R_PCREL       = (int)(objabi.R_PCREL)
	// R_TLS_LE, used on 386, amd64, and ARM, resolves to the offset of the
	// thread-local symbol from the thread local base and is used to implement the
	// "local exec" model for tls access (r.Sym is not set on intel platforms but is
	// set to a TLS symbol -- runtime.tlsg -- in the linker when externally linking).
	R_TLS_LE = (int)(objabi.R_TLS_LE)
	// R_TLS_IE, used 386, amd64, and ARM resolves to the PC-relative offset to a GOT
	// slot containing the offset from the thread-local symbol from the thread local
	// base and is used to implemented the "initial exec" model for tls access (r.Sym
	// is not set on intel platforms but is set to a TLS symbol -- runtime.tlsg -- in
	// the linker when externally linking).
	R_TLS_IE = (int)(objabi.R_TLS_IE)
	// R_USEIFACE marks a type is converted to an interface in the function this
	// relocation is applied to. The target is a type descriptor.
	// This is a marker relocation (0-sized), for the linker's reachabililty
	// analysis.
	R_USEIFACE = (int)(objabi.R_USEIFACE)
	// R_USEIFACEMETHOD marks an interface method that is used in the function
	// this relocation is applied to. The target is an interface type descriptor.
	// The addend is the offset of the method in the type descriptor.
	// This is a marker relocation (0-sized), for the linker's reachabililty
	// analysis.
	R_USEIFACEMETHOD = (int)(objabi.R_USEIFACEMETHOD)
	// R_METHODOFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	// It is a variant of R_ADDROFF used when linking from the uncommonType of a
	// *rtype, and may be set to zero by the linker if it determines the method
	// text is unreachable by the linked program.
	R_METHODOFF = (int)(objabi.R_METHODOFF)
	// R_ADDRCUOFF resolves to a pointer-sized offset from the start of the
	// symbol's DWARF compile unit.
	R_ADDRCUOFF = (int)(objabi.R_ADDRCUOFF)

	R_GOTPCREL = (int)(objabi.R_GOTPCREL)

	R_ARM64_GOTPCREL = (int)(objabi.R_ARM64_GOTPCREL)
)

const (
	//not used, only adapter golang higher version
	R_INITORDER          = 0x10000000 - 9
	R_ARM64_PCREL_LDST8  = 0x10000000 - 8
	R_ARM64_PCREL_LDST16 = 0x10000000 - 7
	R_ARM64_PCREL_LDST32 = 0x10000000 - 6
	R_ARM64_PCREL_LDST64 = 0x10000000 - 5
	R_USETYPE            = 0x10000000 - 4
	R_WEAKADDR           = 0x20000000
	R_WEAK               = 0x8000
)

func RelocTypeString(relocType int) string {
	return (objabi.RelocType)(relocType).String()
}
