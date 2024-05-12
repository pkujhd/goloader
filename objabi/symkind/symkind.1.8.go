//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package symkind

import "cmd/objfile/obj"

// copy from $GOROOT/src/cmd/internal/obj/link.go
const (
	// An otherwise invalid zero value for the type
	Sxxx = int(obj.Sxxx)
	// Executable instructions
	STEXT = int(obj.STEXT)
	// Read only static data
	SRODATA = int(obj.SRODATA)
	// Static data that does not contain any pointers
	SNOPTRDATA = int(obj.SNOPTRDATA)
	// Static data
	SDATA = int(obj.SDATA)
	// Statically data that is initially all 0s
	SBSS = int(obj.SBSS)
	// Statically data that is initially all 0s and does not contain pointers
	SNOPTRBSS = int(obj.SNOPTRBSS)
	// Thread-local data that is initially all 0s
	STLSBSS = int(obj.STLSBSS)
)

func SymKindString(symKind int) string {
	return (obj.SymKind)(symKind).String()
}
