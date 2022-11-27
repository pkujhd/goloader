//go:build darwin
// +build darwin

package goloader

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"syscall"
	"unsafe"
)

func init() {
	// This is somewhat insane, but hey ¯\_(ツ)_/¯
	havePatched, err := PatchMachoSelfMakeWriteable()
	if err != nil {
		panic(err)
	}
	if havePatched {
		// Replace ourselves with the newly patched binary
		// Since this is inside the init function, there shouldn't be too much program state built up...
		log.Printf("patched Mach-O __TEXT segment to make writeable, restarting\n")
		err = syscall.Exec(os.Args[0], os.Args[1:], os.Environ())
		if err != nil {
			panic(err)
		}
	}
}

const Write = 2

const (
	fileHeaderSize32 = 7 * 4
	fileHeaderSize64 = 8 * 4
)

func PatchMachoSelfMakeWriteable() (bool, error) {
	path, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("could not find executable path: %w", err)
	}

	r, err := os.OpenFile(path, os.O_RDWR, 0755)

	if err != nil {
		return false, fmt.Errorf("could not open file %s: %w", path, err)
	}
	sr := io.NewSectionReader(r, 0, 1<<63-1)

	var ident [4]byte
	if _, err := r.ReadAt(ident[0:], 0); err != nil {
		return false, fmt.Errorf("could not read first 4 bytes of file %s: %w", path, err)
	}

	be := binary.BigEndian.Uint32(ident[0:])
	le := binary.LittleEndian.Uint32(ident[0:])
	var magic uint32
	var bo binary.ByteOrder
	switch macho.Magic32 &^ 1 {
	case be &^ 1:
		bo = binary.BigEndian
		magic = be
	case le &^ 1:
		bo = binary.LittleEndian
		magic = le
	default:
		return false, fmt.Errorf("invalid magic number 0x%x", magic)
	}

	header := macho.FileHeader{}
	if err := binary.Read(sr, bo, &header); err != nil {
		return false, fmt.Errorf("could not read macho file header of file %s: %w", path, err)
	}

	offset := int64(fileHeaderSize32)
	if magic == macho.Magic64 {
		offset = fileHeaderSize64
	}

	dat := make([]byte, header.Cmdsz)
	if _, err := r.ReadAt(dat, offset); err != nil {
		return false, fmt.Errorf("failed to read macho command data: %w", err)
	}

	var havePatched = false
	for i := 0; i < int(header.Ncmd); i++ {
		if len(dat) < 8 {
			return false, fmt.Errorf("command block too small")
		}
		cmd, siz := macho.LoadCmd(bo.Uint32(dat[0:4])), bo.Uint32(dat[4:8])
		if siz < 8 || siz > uint32(len(dat)) {
			return false, fmt.Errorf("invalid command block size")
		}
		var cmddat []byte
		cmddat, dat = dat[0:siz], dat[siz:]

		switch cmd {
		case macho.LoadCmdSegment:
			var seg32 macho.Segment32
			b := bytes.NewReader(cmddat)
			if err := binary.Read(b, bo, &seg32); err != nil {
				return false, fmt.Errorf("failed to read LoadCmdSegment: %w", err)
			}
			if cstring(seg32.Name[0:]) == "__TEXT" {
				if seg32.Maxprot&Write == 0 {
					buf := make([]byte, 4)
					_, err := r.ReadAt(buf, offset+int64(unsafe.Offsetof(seg32.Maxprot)))
					if err != nil {
						return false, fmt.Errorf("failed to read MaxProt uint32: %w", err)
					}
					newPerms := bo.Uint32(buf) | Write
					bo.PutUint32(buf, newPerms)
					_, err = r.WriteAt(buf, offset+int64(unsafe.Offsetof(seg32.Maxprot)))
					if err != nil {
						return false, fmt.Errorf("failed to write MaxProt uint32: %w", err)
					}
					if seg32.Maxprot != newPerms {
						havePatched = true
					}
				}
				textIsWriteable = true
			}
		case macho.LoadCmdSegment64:
			var seg64 macho.Segment64
			b := bytes.NewReader(cmddat)
			if err := binary.Read(b, bo, &seg64); err != nil {
				return false, fmt.Errorf("failed to read LoadCmdSegment64: %w", err)
			}
			if cstring(seg64.Name[0:]) == "__TEXT" {
				if seg64.Maxprot&Write == 0 {
					buf := make([]byte, 4)
					_, err := r.ReadAt(buf, offset+int64(unsafe.Offsetof(seg64.Maxprot)))
					if err != nil {
						return false, fmt.Errorf("failed to read MaxProt uint32: %w", err)
					}
					newPerms := bo.Uint32(buf) | Write
					bo.PutUint32(buf, newPerms)
					_, err = r.WriteAt(buf, offset+int64(unsafe.Offsetof(seg64.Maxprot)))
					if err != nil {
						return false, fmt.Errorf("failed to write MaxProt uint32: %w", err)
					}
					if seg64.Maxprot != newPerms {
						havePatched = true
					}
				}
				textIsWriteable = true
			}
		}

		offset += int64(siz)
	}
	return havePatched, nil
}

func cstring(b []byte) string {
	i := bytes.IndexByte(b, 0)
	if i == -1 {
		i = len(b)
	}
	return string(b[0:i])
}
