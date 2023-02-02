//go:build go1.8 && !go1.20
// +build go1.8,!go1.20

package goloader

const (
	TypeImportPathPrefix       = "type..importpath."
	TypeDoubleDotPrefix        = "type.."
	TypePrefix                 = "type."
	ItabPrefix                 = "go.itab."
	TypeStringPrefix           = "go.string."
)
