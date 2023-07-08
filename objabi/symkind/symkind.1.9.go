//go:build go1.9 && !go1.22
// +build go1.9,!go1.22

package symkind

import "cmd/objfile/objabi"

// copy from $GOROOT/src/cmd/internal/objabi/symkind.go
const (
	// An otherwise invalid zero value for the type
	Sxxx = int(objabi.Sxxx)
	// Executable instructions
	STEXT = int(objabi.STEXT)
	// Read only static data
	SRODATA = int(objabi.SRODATA)
	// Static data that does not contain any pointers
	SNOPTRDATA = int(objabi.SNOPTRDATA)
	// Static data
	SDATA = int(objabi.SDATA)
	// Statically data that is initially all 0s
	SBSS = int(objabi.SBSS)
	// Statically data that is initially all 0s and does not contain pointers
	SNOPTRBSS = int(objabi.SNOPTRBSS)
	// Thread-local data that is initally all 0s
	STLSBSS = int(objabi.STLSBSS)
)

func SymKindString(symKind int) string {
	return (objabi.SymKind)(symKind).String()
}
