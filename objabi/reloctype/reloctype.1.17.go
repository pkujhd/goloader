//go:build go1.17 && !go1.20
// +build go1.17,!go1.20

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
	R_ADDROFF   = (int)(objabi.R_ADDROFF)
	R_CALL      = (int)(objabi.R_CALL)
	R_CALLARM   = (int)(objabi.R_CALLARM)
	R_CALLARM64 = (int)(objabi.R_CALLARM64)
	R_CALLIND   = (int)(objabi.R_CALLIND)
	R_PCREL     = (int)(objabi.R_PCREL)
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
	R_TLS_IE   = (int)(objabi.R_TLS_IE)
	R_USEFIELD = (int)(objabi.R_USEFIELD)
	// R_USETYPE resolves to an *rtype, but no relocation is created. The
	// linker uses this as a signal that the pointed-to type information
	// should be linked into the final binary, even if there are no other
	// direct references. (This is used for types reachable by reflection.)
	R_USETYPE = (int)(objabi.R_USETYPE)
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

	// R_KEEP tells the linker to keep the referred-to symbol in the final binary
	// if the symbol containing the R_KEEP relocation is in the final binary.
	R_KEEP = (int)(objabi.R_KEEP)

	R_GOTPCREL = (int)(objabi.R_GOTPCREL)

	// Set a MOV[NZ] immediate field to bits [15:0] of the offset from the thread
	// local base to the thread local variable defined by the referenced (thread
	// local) symbol. Error if the offset does not fit into 16 bits.
	R_ARM64_TLS_LE = (int)(objabi.R_ARM64_TLS_LE)

	// Relocates an ADRP; LD64 instruction sequence to load the offset between
	// the thread local base and the thread local variable defined by the
	// referenced (thread local) symbol from the GOT.
	R_ARM64_TLS_IE = int(objabi.R_ARM64_TLS_IE)

	// R_ARM64_GOTPCREL relocates an adrp, ld64 pair to compute the address of the GOT
	// slot of the referenced symbol.
	R_ARM64_GOTPCREL = (int)(objabi.R_ARM64_GOTPCREL)

	R_WEAK = 0x8000

	R_WEAKADDR    = R_WEAK | R_ADDR
	R_WEAKADDROFF = R_WEAK | R_ADDROFF
)

const (
	//not used, only adapter golang higher version
	R_ARM64_PCREL_LDST8  = 0x10000000 - 8
	R_ARM64_PCREL_LDST16 = 0x10000000 - 7
	R_ARM64_PCREL_LDST32 = 0x10000000 - 6
	R_ARM64_PCREL_LDST64 = 0x10000000 - 5
)
