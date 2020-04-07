// +build go1.8
// +build !go1.12,!go1.13,!go1.14,!go1.15

package goloader

const (
	R_PCREL = 15
	// R_TLS_LE, used on 386, amd64, and ARM, resolves to the offset of the
	// thread-local symbol from the thread local base and is used to implement the
	// "local exec" model for tls access (r.Sym is not set on intel platforms but is
	// set to a TLS symbol -- runtime.tlsg -- in the linker when externally linking).
	R_TLS_LE = 16
	// R_METHODOFF resolves to a 32-bit offset from the beginning of the section
	// holding the data being relocated to the referenced symbol.
	// It is a variant of R_ADDROFF used when linking from the uncommonType of a
	// *rtype, and may be set to zero by the linker if it determines the method
	// text is unreachable by the linked program.
	R_METHODOFF = 24
)

func addStackObject(code *CodeReloc, fi *funcInfoData, seg *segment, symPtr map[string]uintptr) {
}

func addDeferReturn(code *CodeReloc, fi *funcInfoData, seg *segment) {
}
