//go:build go1.20 && !go1.23
// +build go1.20,!go1.23

package goloader

const (
	TypeImportPathPrefix = "type:.importpath."
	TypeDoubleDotPrefix  = "type:."
	TypePrefix           = "type:"
	ItabPrefix           = "go:itab."
	TypeStringPrefix     = "go:string."
	ObjSymbolSeparator   = ":"
)
