package goloader

import (
	"encoding/binary"

	"github.com/pkujhd/goloader/obj"
)

func findFileTab(filename string, namemap map[string]int, filetab []uint32) int32 {
	tab := namemap[filename]
	for index, value := range filetab {
		if uint32(tab) == value {
			return int32(index)
		}
	}
	return -1
}

func rewritePCFile(symbol *obj.ObjSymbol, linker *Linker) {
	p := symbol.Func.PCFile
	pcfile := make([]byte, 0)
	pc := uintptr(0)
	lastpc := uintptr(0)
	val := int32(-1)
	lastval := int32(-1)
	var ok bool
	p, ok = step(p, &pc, &val, true)
	for {
		if !ok || len(p) <= 0 {
			break
		}
		buf := make([]byte, 32)
		nval := findFileTab(symbol.Func.File[val], linker.nameMap, linker.filetab)
		n := binary.PutVarint(buf, int64(nval-lastval))
		pcfile = append(pcfile, buf[:n]...)
		n = binary.PutUvarint(buf, uint64(pc-lastpc))
		pcfile = append(pcfile, buf[:n]...)

		lastpc = pc
		lastval = nval
		p, ok = step(p, &pc, &val, false)
	}

	pcfile = append(pcfile, 0)
	symbol.Func.PCFile = pcfile
}
