package goloader

import (
	"cmd/objfile/sys"
	"encoding/binary"
	"fmt"

	"github.com/pkujhd/goloader/obj"
)

func dumpPCValue(b []byte, prefix string) {
	fmt.Println(prefix, b)
	var pc uintptr
	val := int32(-1)
	var ok bool
	b, ok = step(b, &pc, &val, true)
	for {
		if !ok || len(b) <= 0 {
			fmt.Println(prefix, "step end")
			break
		}
		fmt.Println(prefix, "pc:", pc, "val:", val)
		b, ok = step(b, &pc, &val, false)
	}
}

func writePCValue(p []byte, val int64, pc uint64) []byte {
	buf := make([]byte, 32)
	n := binary.PutVarint(buf, val)
	p = append(p, buf[:n]...)
	n = binary.PutUvarint(buf, pc)
	p = append(p, buf[:n]...)
	return p
}

func rewritePCFile(symbol *obj.ObjSymbol, linker *Linker) {
	// golang version <= 1.15 PCFile need rewrite, PCFile (pc, val), val only adapte symbol.Func.File
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
		nval := obj.FindFileTab(symbol.Func.File[val], linker.NameMap, linker.Filetab)
		pcfile = writePCValue(pcfile, int64(nval-lastval), uint64(pc-lastpc))
		lastpc = pc
		lastval = nval
		p, ok = step(p, &pc, &val, false)
	}
	pcfile = append(pcfile, 0)
	symbol.Func.PCFile = pcfile
}

func pcValue(p []byte, targetPC uintptr) (int32, uintptr) {
	startPC := uintptr(0)
	pc := uintptr(0)
	val := int32(-1)
	prevpc := pc
	for {
		var ok bool
		p, ok = step(p, &pc, &val, pc == startPC)
		if !ok || len(p) == 0 {
			break
		}
		if targetPC < pc {
			return val, prevpc
		}
		prevpc = pc
	}
	return -1, InvalidHandleValue
}

func updateLastPCValue(pcVals *[]byte, nval int32, npc, pcQuantum uintptr) {
	p := *pcVals
	pc := uintptr(0)
	lastpc := uintptr(0)
	val := int32(-1)
	lastval := int32(-1)
	npcVals := make([]byte, 0)
	for {
		var ok bool
		p, ok = step(p, &pc, &val, pc == 0)
		if len(p) == 1 && p[0] == 0 && val == nval {
			npcVals = writePCValue(npcVals, int64(nval-lastval), uint64((npc-lastpc)/pcQuantum))
			break
		}
		if !ok || len(p) == 0 {
			npcVals = writePCValue(npcVals, int64(nval-lastval), uint64((npc-lastpc)/pcQuantum))
			break
		}
		npcVals = writePCValue(npcVals, int64(val-lastval), uint64((pc-lastpc)/pcQuantum))
		lastpc = pc
		lastval = val
	}
	npcVals = append(npcVals, 0)
	*pcVals = npcVals
}

func patchPCValues(linker *Linker, pcVals *[]byte, reloc obj.Reloc) {
	// Use the pcvalue at the offset of the reloc for the entire of that reloc's epilogue.
	// This ensures that if the code is pre-empted or the stack unwound while we're inside the epilogue, the runtime behaves correctly
	if len(*pcVals) == 0 {
		return
	}
	var pcQuantum uintptr = 1
	if linker.Arch.Family == sys.ARM64 {
		pcQuantum = 4
	}
	val, startPC := pcValue(*pcVals, uintptr(reloc.Offset))
	if startPC == InvalidHandleValue && val == -1 {
		panic(fmt.Sprintf("couldn't interpret pcvalue data with pc offset: %d", reloc.Offset))
	}
	updateLastPCValue(pcVals, val, uintptr(reloc.Epilogue.Offset+reloc.Epilogue.Size), pcQuantum)
}
