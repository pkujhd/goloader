package goloader

import (
	"cmd/objfile/gcprog"
	"fmt"
	"sort"
	"unsafe"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/symkind"
)

const (
	KindGCProg = 1 << 6
)

func generategcdata(linker *Linker, codeModule *CodeModule, symbolMap map[string]uintptr, w *gcprog.Writer, sym *obj.Sym) error {
	segment := &codeModule.segment
	//if symbol is in loader, ignore generate gc data
	if symbolMap[sym.Name] < uintptr(segment.dataBase) || symbolMap[sym.Name] > uintptr(segment.dataBase+segment.sumDataLen) {
		return nil
	}
	typeName := linker.objsymbolMap[sym.Name].Type
	sval := int64(symbolMap[sym.Name] - uintptr(segment.dataBase))
	if int(sym.Kind) == symkind.SBSS {
		sval = sval - int64(segment.dataLen+segment.noptrdataLen)
	}
	if ptr, ok := symbolMap[typeName]; ok {
		typ := (*_type)(adduintptr(ptr, 0))
		nptr := int64(typ.ptrdata) / int64(linker.Arch.PtrSize)
		if typ.kind&KindGCProg == 0 {
			var mask []byte
			append2Slice(&mask, uintptr(unsafe.Pointer(typ.gcdata)), int(nptr+7)/8)
			for i := int64(0); i < nptr; i++ {
				if (mask[i/8]>>uint(i%8))&1 != 0 {
					w.Ptr(sval/int64(linker.Arch.PtrSize) + i)
				}
			}

		} else {
			var prog []byte
			append2Slice(&prog, uintptr(unsafe.Pointer(typ.gcdata)), Uint32Size+int((*(*uint32)(unsafe.Pointer(typ.gcdata)))))
			w.ZeroUntil(sval / int64(linker.Arch.PtrSize))
			w.Append(prog[4:], nptr)
		}
	} else {
		return fmt.Errorf("type: %s not found\n", typeName)
	}
	return nil
}

func sortSym(symMap map[string]*obj.Sym, kind int) []*obj.Sym {
	syms := make(map[int]*obj.Sym)
	keys := []int{}
	for _, sym := range symMap {
		if sym.Kind == kind {
			syms[sym.Offset] = sym
			keys = append(keys, sym.Offset)
		}
	}
	sort.Ints(keys)
	symbols := []*obj.Sym{}
	for _, key := range keys {
		symbols = append(symbols, syms[key])
	}
	return symbols
}

func (linker *Linker) addgcdata(codeModule *CodeModule, symbolMap map[string]uintptr) error {
	module := codeModule.module
	w := gcprog.Writer{}
	w.Init(func(x byte) {
		codeModule.gcdata = append(codeModule.gcdata, x)
	})
	for _, sym := range sortSym(linker.symMap, symkind.SDATA) {
		err := generategcdata(linker, codeModule, symbolMap, &w, sym)
		if err != nil {
			return err
		}
	}
	w.ZeroUntil(int64(module.edata-module.data) / int64(linker.Arch.PtrSize))
	w.End()
	module.gcdata = (*sliceHeader)(unsafe.Pointer(&codeModule.gcdata)).Data
	module.gcdatamask = progToPointerMask((*byte)(adduintptr(module.gcdata, 0)), module.edata-module.data)

	w = gcprog.Writer{}
	w.Init(func(x byte) {
		codeModule.gcbss = append(codeModule.gcbss, x)
	})

	for _, sym := range sortSym(linker.symMap, symkind.SBSS) {
		err := generategcdata(linker, codeModule, symbolMap, &w, sym)
		if err != nil {
			return err
		}
	}
	w.ZeroUntil(int64(module.ebss-module.bss) / int64(linker.Arch.PtrSize))
	w.End()
	module.gcbss = (*sliceHeader)(unsafe.Pointer(&codeModule.gcbss)).Data
	module.gcbssmask = progToPointerMask((*byte)(adduintptr(module.gcbss, 0)), module.ebss-module.bss)
	return nil
}
