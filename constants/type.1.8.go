//go:build go1.8 && !go1.20
// +build go1.8,!go1.20

package constants

const (
	TypeImportPathPrefix = "type..importpath."
	TypeNameDataPrefix   = "type..namedata."
	TypeDoubleDotPrefix  = "type.."
	TypePrefix           = "type."
	ItabPrefix           = "go.itab."
	TypeStringPrefix     = "go.string."
)
