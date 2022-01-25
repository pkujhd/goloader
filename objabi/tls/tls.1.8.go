//go:build go1.8 && !go1.11
// +build go1.8,!go1.11

package tls

import (
	"cmd/objfile/sys"
	"fmt"
	"runtime"
)

//see:src/cmd/link/internal/ld/sym.go
func GetTLSOffset(arch *sys.Arch, ptrsize int) uintptr {
	switch GetHeadType() {
	case Hwindows:
		return 0x0
	/*
	 * ELF uses TLS offset negative from FS.
	 * Translate 0(FS) and 8(FS) into -16(FS) and -8(FS).
	 * Known to low-level assembly in package runtime and runtime/cgo.
	 */
	case Hlinux,
		Hfreebsd,
		Hnetbsd,
		Hopenbsd,
		Hdragonfly,
		Hsolaris:
		if runtime.GOOS == "android" {
			switch arch.Name {
			case sys.ArchAMD64.Name:
				// Android/amd64 constant - offset from 0(FS) to our TLS slot.
				// Explained in src/runtime/cgo/gcc_android_*.c
				return 0x1d0
			case sys.Arch386.Name:
				// Android/386 constant - offset from 0(GS) to our TLS slot.
				return 0xf8
			default:
				offset := -1 * ptrsize
				return uintptr(offset)
			}
		}
		offset := -1 * ptrsize
		return uintptr(offset)
	/*
	 * OS X system constants - offset from 0(GS) to our TLS.
	 * Explained in src/runtime/cgo/gcc_darwin_*.c.
	 */
	case Hdarwin:
		switch arch.Name {
		case sys.Arch386.Name:
			return uintptr(0x468)
		case sys.ArchAMD64.Name:
			return uintptr(0x8a0)
		case sys.ArchARM64.Name,
			sys.ArchARM.Name:
			return 0x0
		default:
			panic(fmt.Sprintf("unknown thread-local storage offset for darwin/%s", arch.Name))
		}
	default:
		panic(fmt.Sprintf("undealed regTLS on os:%s", runtime.GOOS))
	}
}
