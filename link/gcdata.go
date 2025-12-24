package link

import (
	"cmd/objfile/gcprog"
	"fmt"
	"sort"
	"unsafe"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/symkind"
)

func generateGcData(linker *Linker, codeModule *CodeModule, symbolMap map[string]uintptr, w *gcprog.Writer, sym *obj.Sym) error {
	segment := &codeModule.segment
	//if symbol is in loader, ignore generate gc data
	if symbolMap[sym.Name] < uintptr(segment.dataBase) || symbolMap[sym.Name] > uintptr(segment.dataBase+segment.dataSeg.length) {
		return nil
	}
	objsym := linker.SymMap[sym.Name]
	typeName := objsym.Type
	if len(typeName) == 0 {
		// This is likely a global var with no type information encoded, so can't be GC'd (ignore it)
		return nil
	}
	off := int64(symbolMap[sym.Name] - uintptr(segment.dataBase))
	if sym.Kind == symkind.SBSS {
		off = off - int64(segment.dataLen+segment.noptrdataLen)
	}
	if ptr, ok := symbolMap[typeName]; ok {
		typ := (*_type)(adduintptr(ptr, 0))
		gcDataAddType(linker, w, off, typ)
	} else {
		return fmt.Errorf("type:%s not found\n", typeName)
	}
	return nil
}

func sortSym(symMap map[string]*obj.Sym, kindFunc func(k int) bool) []*obj.Sym {
	symbolMaps := make(map[int]*obj.Sym)
	keys := []int{}
	for _, sym := range symMap {
		if kindFunc(sym.Kind) {
			symbolMaps[sym.Offset] = sym
			keys = append(keys, sym.Offset)
		}
	}
	sort.Ints(keys)
	symbols := make([]*obj.Sym, 0)
	for _, key := range keys {
		symbols = append(symbols, symbolMaps[key])
	}
	return symbols
}

func (linker *Linker) addgcdata(codeModule *CodeModule, symbolMap map[string]uintptr) error {
	module := codeModule.module
	w := gcprog.Writer{}
	w.Init(func(x byte) {
		codeModule.gcdata = append(codeModule.gcdata, x)
	})
	for _, sym := range sortSym(linker.SymMap, symkind.IsData) {
		err := generateGcData(linker, codeModule, symbolMap, &w, sym)
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

	for _, sym := range sortSym(linker.SymMap, symkind.IsBss) {
		err := generateGcData(linker, codeModule, symbolMap, &w, sym)
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
