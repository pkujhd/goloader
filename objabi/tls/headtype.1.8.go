//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package tls

import (
	"cmd/objfile/obj"
	"runtime"
)

type HeadType uint8

const (
	Hunknown   = uint8(obj.Hunknown)
	Hdarwin    = uint8(obj.Hdarwin)
	Hdragonfly = uint8(obj.Hdragonfly)
	Hfreebsd   = uint8(obj.Hfreebsd)
	Hlinux     = uint8(obj.Hlinux)
	Hnetbsd    = uint8(obj.Hnetbsd)
	Hopenbsd   = uint8(obj.Hopenbsd)
	Hplan9     = uint8(obj.Hplan9)
	Hsolaris   = uint8(obj.Hsolaris)
	Hwindows   = uint8(obj.Hwindows)
)

func GetHeadType() uint8 {
	var h obj.HeadType
	h.Set(runtime.GOOS)
	return uint8(h)
}
