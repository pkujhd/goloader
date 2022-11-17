package jit

import (
	"fmt"
	"github.com/pkujhd/goloader"
	"reflect"
	"unsafe"
)

type LoadableUnit struct {
	Linker               *goloader.Linker
	ImportPath           string
	ParsedFiles          []*ParsedFile
	SymbolTypeFuncLookup map[string]string
}

func (l *LoadableUnit) Load() (module *goloader.CodeModule, functions map[string]interface{}, err error) {
	if l == nil {
		return nil, nil, fmt.Errorf("can't load nil LoadableUnit")
	}
	module, err = goloader.Load(l.Linker, globalSymPtr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load linker: %w", err)
	}

	functions = map[string]interface{}{}
	for symName, lookupFuncName := range l.SymbolTypeFuncLookup {
		symPtr := module.Syms[l.ImportPath+"."+symName]
		lookupFunc := module.Syms[l.ImportPath+"."+lookupFuncName]
		if symPtr == 0 {
			return nil, nil, fmt.Errorf("failed to find symbol %s (have: %v)", l.ImportPath+"."+symName, module.Syms)
		}
		if lookupFunc == 0 {
			return nil, nil, fmt.Errorf("failed to find type lookup function %s (have: %v)", l.ImportPath+"."+lookupFuncName, module.Syms)
		}
		typeCheckerContainer := (uintptr)(unsafe.Pointer(&lookupFunc))
		typeCheckerFunc := *(*func() reflect.Type)(unsafe.Pointer(&typeCheckerContainer))
		symbolType := typeCheckerFunc()
		eface := *(*emptyInterface)(unsafe.Pointer(&symbolType))

		rtype := eface.word
		var val interface{}
		valp := (*[2]unsafe.Pointer)(unsafe.Pointer(&val))
		(*valp)[0] = rtype
		(*valp)[1] = unsafe.Pointer(&symPtr)
		functions[symName] = val
	}
	return module, functions, nil
}
