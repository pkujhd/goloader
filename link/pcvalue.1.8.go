//go:build go1.8 && !go1.16
// +build go1.8,!go1.16

package link

import (
	"github.com/pkujhd/goloader/obj"
)

func adaptPCFile(linker *Linker, symbol *obj.ObjSymbol) {
	// golang version <= 1.15 PCFile need rewrite, PCFile (pc, val), val only adapte symbol.Func.File
	p := symbol.Func.PCFile
	pcfile := make([]byte, 0)
	pc := uintptr(0)
	lastpc := uintptr(0)
	val := int32(-1)
	lastVal := int32(-1)
	var ok bool
	p, ok = step(p, &pc, &val, true)
	for {
		if !ok || len(p) <= 0 {
			break
		}
		nVal := obj.FindFileTab(symbol.Func.File[val], linker.NameMap, linker.Filetab)
		pcfile = writePCValue(pcfile, int64(nVal-lastVal), uint64(pc-lastpc))
		lastpc = pc
		lastVal = nVal
		p, ok = step(p, &pc, &val, false)
	}
	pcfile = append(pcfile, 0)
	symbol.Func.PCFile = pcfile
}
