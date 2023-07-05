//go:build go1.20 && !go1.22
// +build go1.20,!go1.22

package obj

const (
	TypeStringPrefix    = "go:string."
	ObjSymbolSeparator  = ":"
	TypeDoubleDotPrefix = "type:."
	TypePrefix          = "type:"
)
