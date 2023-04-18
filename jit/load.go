package jit

import (
	"fmt"
	"github.com/eh-steve/goloader"
)

type LoadableUnit struct {
	Linker     *goloader.Linker
	ImportPath string
	Module     *goloader.CodeModule
	Package    *Package
}

func (l *LoadableUnit) Load() (module *goloader.CodeModule, err error) {
	if l == nil || l.Linker == nil {
		return nil, fmt.Errorf("can't load nil LoadableUnit")
	}
	module, err = goloader.Load(l.Linker, globalSymPtr)
	if err != nil {
		return nil, fmt.Errorf("failed to load linker: %w", err)
	}

	l.Module = module

	return module, nil
}
