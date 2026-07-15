//go:build go1.8 && !go1.27
// +build go1.8,!go1.27

package link

import (
	"github.com/pkujhd/goloader/constants"
)

// !IMPORTANT: only init firstmodule type, avoid load multiple objs but unload non-sequence errors
func typelinksRegister(symPtr map[string]uintptr) {
	md := firstmoduledata
	for _, tl := range md.typelinks {
		t := (*_type)(adduintptr(md.types, int(tl)))
		registerType(t, symPtr)
	}
}

func (linker *Linker) AddTypeLink(codeModule *CodeModule) {
	module := codeModule.module
	for name, symbol := range linker.SymMap {
		if isTypeName(name) && symbol.Offset != constants.InvalidOffset {
			typeOff := int32(codeModule.dataBase + symbol.Offset - int(module.types))
			module.typelinks = append(module.typelinks, typeOff)
		}
	}
}

func removeModuleToTypelinks(md *moduledata) {}
