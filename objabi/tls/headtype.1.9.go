//go:build go1.9 && !go1.21
// +build go1.9,!go1.21

package tls

import (
	"cmd/objfile/objabi"
	"runtime"
)

const (
	Hunknown   = uint8(objabi.Hunknown)
	Hdarwin    = uint8(objabi.Hdarwin)
	Hdragonfly = uint8(objabi.Hdragonfly)
	Hfreebsd   = uint8(objabi.Hfreebsd)
	Hlinux     = uint8(objabi.Hlinux)
	Hnetbsd    = uint8(objabi.Hnetbsd)
	Hopenbsd   = uint8(objabi.Hopenbsd)
	Hplan9     = uint8(objabi.Hplan9)
	Hsolaris   = uint8(objabi.Hsolaris)
	Hwindows   = uint8(objabi.Hwindows)
)

func GetHeadType() uint8 {
	var h objabi.HeadType
	h.Set(runtime.GOOS)
	return uint8(h)
}
