//go:build go1.20 && !go1.25
// +build go1.20,!go1.25

package constants

const (
	TypeImportPathPrefix = "type:.importpath."
	TypeNameDataPrefix   = "type:.namedata."
	TypeDoubleDotPrefix  = "type:."
	TypePrefix           = "type:"
	ItabPrefix           = "go:itab."
	TypeStringPrefix     = "go:string."
)
