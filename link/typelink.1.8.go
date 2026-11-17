//go:build go1.8 && !go1.27
// +build go1.8,!go1.27

package link

import (
	"unsafe"

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

func removeModuleToTypelinks(md *moduledata) {
	// The tested Go 1.23.2 runtime.typelinksinit pins every module typemap in the
	// runtime.pinnedTypemaps slice. The stock goloader implementation leaves
	// these maps pinned forever, even after moduledata is removed. Remove the
	// exact map belonging to this module before unmapping its data segment.
	if runtimePinnedTypemapsAddr == 0 || md == nil || md.typemap == nil {
		return
	}
	pinned := (*[]map[typeOff]*_type)(unsafe.Pointer(runtimePinnedTypemapsAddr))
	maps := *pinned
	target := *(*uintptr)(unsafe.Pointer(&md.typemap))
	for i := range maps {
		current := *(*uintptr)(unsafe.Pointer(&maps[i]))
		if current == target {
			copy(maps[i:], maps[i+1:])
			maps[len(maps)-1] = nil
			*pinned = maps[:len(maps)-1]
			md.typemap = nil
			return
		}
	}
}
