//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package reloctype

import "cmd/objfile/obj"

// copy from $GOROOT/src/cmd/internal/obj/link.go
const (
	R_ADDR = (int)(obj.R_ADDR)
	// R_ADDRARM64 relocates an adrp, add pair to compute the address of the
	// referenced symbol.
	R_ADDRARM64 = (int)(obj.R_ADDRARM64)
	// R_ADDROFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	R_ADDROFF = (int)(obj.R_ADDROFF)
	// R_WEAKADDROFF resolves just like R_ADDROFF but is a weak relocation.
	// A weak relocation does not make the symbol it refers to reachable,
	// and is only honored by the linker if the symbol is in some other way
	// reachable.
	R_WEAKADDROFF = (int)(obj.R_WEAKADDROFF)
	R_CALL        = (int)(obj.R_CALL)
	R_CALLARM     = (int)(obj.R_CALLARM)
	R_CALLARM64   = (int)(obj.R_CALLARM64)
	R_CALLIND     = (int)(obj.R_CALLIND)
	R_PCREL       = (int)(obj.R_PCREL)
	// R_TLS_LE, used on 386, amd64, and ARM, resolves to the offset of the
	// thread-local symbol from the thread local base and is used to implement the
	// "local exec" model for tls access (r.Sym is not set on intel platforms but is
	// set to a TLS symbol -- runtime.tlsg -- in the linker when externally linking).
	R_TLS_LE = (int)(obj.R_TLS_LE)
	// R_TLS_IE, used 386, amd64, and ARM resolves to the PC-relative offset to a GOT
	// slot containing the offset from the thread-local symbol from the thread local
	// base and is used to implemented the "initial exec" model for tls access (r.Sym
	// is not set on intel platforms but is set to a TLS symbol -- runtime.tlsg -- in
	// the linker when externally linking).
	R_TLS_IE = (int)(obj.R_TLS_IE)
	// R_METHODOFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	// It is a variant of R_ADDROFF used when linking from the uncommonType of a
	// *rtype, and may be set to zero by the linker if it determines the method
	// text is unreachable by the linked program.
	R_METHODOFF = (int)(obj.R_METHODOFF)

	R_GOTPCREL = (int)(obj.R_GOTPCREL)

	R_ARM64_GOTPCREL = (int)(obj.R_ARM64_GOTPCREL)
)

const (
	//not used, only adapter golang higher version
	R_KEEP               = 0x10000000 - 10
	R_INITORDER          = 0x10000000 - 9
	R_ARM64_PCREL_LDST8  = 0x10000000 - 8
	R_ARM64_PCREL_LDST16 = 0x10000000 - 7
	R_ARM64_PCREL_LDST32 = 0x10000000 - 6
	R_ARM64_PCREL_LDST64 = 0x10000000 - 5
	R_USETYPE            = 0x10000000 - 4
	R_USEIFACE           = 0x10000000 - 3
	R_USEIFACEMETHOD     = 0x10000000 - 2
	R_ADDRCUOFF          = 0x10000000 - 1
	R_WEAKADDR           = 0x20000000
	R_WEAK               = 0x8000
)

func RelocTypeString(relocType int) string {
	return (obj.RelocType)(relocType).String()
}
