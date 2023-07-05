//go:build go1.20 && !go1.22
// +build go1.20,!go1.22

package goloader

const (
	TypeImportPathPrefix = "type:.importpath."
	TypeDoubleDotPrefix  = "type:."
	TypePrefix           = "type:"
	ItabPrefix           = "go:itab."
	TypeStringPrefix     = "go:string."
	ObjSymbolSeparator   = ":"
)
