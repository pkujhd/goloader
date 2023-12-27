//go:build go1.20 && !go1.23
// +build go1.20,!go1.23

package constants

const (
	TypeImportPathPrefix = "type:.importpath."
	TypeDoubleDotPrefix  = "type:."
	TypePrefix           = "type:"
	ItabPrefix           = "go:itab."
	TypeStringPrefix     = "go:string."
)
