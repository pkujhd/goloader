//go:build go1.20 && !go1.23
// +build go1.20,!go1.23

package obj

const (
	TypeStringPrefix    = "go:string."
	ObjSymbolSeparator  = ":"
	TypeDoubleDotPrefix = "type:."
	TypePrefix          = "type:"
)
