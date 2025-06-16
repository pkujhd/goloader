//go:build go1.24 && !go1.26
// +build go1.24,!go1.26

package symkind

import "cmd/objfile/objabi"

// copy from $GOROOT/src/cmd/internal/objabi/symkind.go
const (
	// An otherwise invalid zero value for the type
	Sxxx = int(objabi.Sxxx)
	// Executable instructions
	STEXT     = int(objabi.STEXT)
	STEXTFIPS = int(objabi.STEXTFIPS)
	// Read only static data
	SRODATA     = int(objabi.SRODATA)
	SRODATAFIPS = int(objabi.SRODATAFIPS)
	// Static data that does not contain any pointers
	SNOPTRDATA     = int(objabi.SNOPTRDATA)
	SNOPTRDATAFIPS = int(objabi.SNOPTRDATAFIPS)
	// Static data
	SDATA     = int(objabi.SDATA)
	SDATAFIPS = int(objabi.SDATAFIPS)
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
