package goloader

import (
	"fmt"
	"github.com/pkujhd/goloader/mprotect"
	"unsafe"
)

// Similar to runtime.(*itab).init() but replaces method text pointers to start the offset from the specified base address
func (m *itab) adjustMethods(codeBase uintptr, methodIndices []int) string {
	inter := m.inter
	typ := m._type
	x := typ.uncommon()

	ni := len(inter.mhdr)
	nt := int(x.mcount)
	xmhdr := (*[1 << 16]method)(add(unsafe.Pointer(x), uintptr(x.moff)))[:nt:nt]
	methods := (*[1 << 16]unsafe.Pointer)(unsafe.Pointer(&m.fun[0]))[:ni:ni]
imethods:
	for k := 0; k < ni; k++ {
		i := &inter.mhdr[k]
		itype := inter.typ.typeOff(i.ityp)
		name := inter.typ.nameOff(i.name)
		iname := name.name()
		ipkg := name.pkgPath()
		if ipkg == "" {
			ipkg = inter.pkgpath.name()
		}
		for _, j := range methodIndices {
			t := &xmhdr[j]
			if t.ifn < 0 {
				panic("shouldn't be possible")
			}
			tname := typ.nameOff(t.name)
			if typ.typeOff(t.mtyp) == itype && tname.name() == iname {
				pkgPath := tname.pkgPath()
				if pkgPath == "" {
					pkgPath = typ.nameOff(x.pkgpath).name()
				}
				if tname.isExported() || pkgPath == ipkg {
					if m != nil {
						ifn := unsafe.Pointer(codeBase + uintptr(t.ifn))
						methods[k] = ifn
					}
					continue imethods
				}
			}
		}
	}
	return ""
}

func patchTypeMethods(t *_type, u, prevU *uncommonType, patchedTypeMethodsIfn, patchedTypeMethodsTfn map[*_type][]int) (err error) {
	// It's possible that a baked in type in the main module does not have all its methods reachable
	// (i.e. some method offsets will be set to -1 via the linker's reachability analysis) whereas the
	// new type will have them them all.

	// In this case, to avoid fatal "unreachable method called. linker bug?" errors, we need to
	// manipulate the method offsets to make them not -1, and manually partially adjust the
	// firstmodule itabs to rewrite the method addresses to point at the new module text (and remember to clean up afterwards)

	if u != nil && prevU != nil {
		methods := u.methods()
		prevMethods := prevU.methods()
		if len(methods) == len(prevMethods) {
			for i := range methods {
				if methods[i].tfn == -1 || methods[i].ifn == -1 {

					if prevMethods[i].ifn != -1 {
						page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].ifn)))
						err = mprotect.MprotectMakeWritable(page)
						if err != nil {
							return fmt.Errorf("failed to make page writeable while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}
						methods[i].ifn = prevMethods[i].ifn
						err = mprotect.MprotectMakeReadOnly(page)
						if err != nil {
							return fmt.Errorf("failed to make page read only while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}
						// Store for later cleanup on Unload()
						patchedTypeMethodsIfn[t] = append(patchedTypeMethodsIfn[t], i)
					}

					if prevMethods[i].tfn != -1 {
						page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].tfn)))
						err = mprotect.MprotectMakeWritable(page)
						if err != nil {
							return fmt.Errorf("failed to make page writeable while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}
						methods[i].tfn = prevMethods[i].tfn
						err = mprotect.MprotectMakeReadOnly(page)
						if err != nil {
							return fmt.Errorf("failed to make page read only while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}
						// Store for later cleanup on Unload()
						patchedTypeMethodsTfn[t] = append(patchedTypeMethodsTfn[t], i)
					}
				}
			}
		}
	}
	return nil
}

func (cm *CodeModule) patchTypeMethods() (err error) {
	// Adjust the main module's itabs so that any missing methods now point to new module's text instead of "unreachable code".

	firstModule := activeModules()[0]

	for _, itab := range firstModule.itablinks {
		methodIndicesIfn, ifnPatched := cm.patchedTypeMethodsIfn[itab._type]
		methodIndicesTfn, tfnPatched := cm.patchedTypeMethodsTfn[itab._type]
		if ifnPatched || tfnPatched {
			page := mprotect.GetPage(uintptr(unsafe.Pointer(&itab.fun[0])))
			err = mprotect.MprotectMakeWritable(page)
			if err != nil {
				return fmt.Errorf("failed to make page writeable while re-initing itab for type %s: %w", _name(itab._type.nameOff(itab._type.str)), err)
			}
			if ifnPatched {
				itab.adjustMethods(uintptr(cm.codeBase), methodIndicesIfn)
			}
			if tfnPatched {
				itab.adjustMethods(uintptr(cm.codeBase), methodIndicesTfn)
			}

			err = mprotect.MprotectMakeReadOnly(page)
			if err != nil {
				return fmt.Errorf("failed to make page read only while re-initing itab for type %s: %w", _name(itab._type.nameOff(itab._type.str)), err)
			}
		}
	}
	return nil
}

func (cm *CodeModule) revertPatchedTypeMethods() error {
	for t, indices := range cm.patchedTypeMethodsIfn {
		u := t.uncommon()
		methods := u.methods()
		for _, i := range indices {
			page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].ifn)))
			err := mprotect.MprotectMakeWritable(page)
			if err != nil {
				return fmt.Errorf("failed to make page writeable while patching type %s: %w", _name(t.nameOff(t.str)), err)
			}
			methods[i].ifn = -1
			err = mprotect.MprotectMakeReadOnly(page)
			if err != nil {
				return fmt.Errorf("failed to make page read only while patching type %s: %w", _name(t.nameOff(t.str)), err)
			}
		}
	}
	for t, indices := range cm.patchedTypeMethodsTfn {
		u := t.uncommon()
		methods := u.methods()
		for _, i := range indices {
			page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].tfn)))
			err := mprotect.MprotectMakeWritable(page)
			if err != nil {
				return fmt.Errorf("failed to make page writeable while patching type %s: %w", _name(t.nameOff(t.str)), err)
			}
			methods[i].tfn = -1
			err = mprotect.MprotectMakeReadOnly(page)
			if err != nil {
				return fmt.Errorf("failed to make page read only while patching type %s: %w", _name(t.nameOff(t.str)), err)
			}
		}
	}
	return nil
}
