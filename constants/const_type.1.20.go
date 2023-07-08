//go:build go1.20 && !go1.21
// +build go1.20,!go1.21

package constants

const (
	TypeImportPathPrefix = "type:.importpath."
	TypeDoubleDotPrefix  = "type:."
	TypePrefix           = "type:"
	ItabPrefix           = "go:itab."
	TypeStringPrefix     = "go:string."
)
