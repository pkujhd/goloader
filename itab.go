package goloader

import (
	"fmt"
	"github.com/eh-steve/goloader/mprotect"
	"runtime"
	"sort"
	"unsafe"
)

// modules is a store of all loaded CodeModules for their patches to apply to the firstmodule's itab methods.
// An itab in the first module can only point to a single copy of a method for a given interface+type pair,
// even if multiple CodeModules have included copies of the same method.
// To prevent the unloading of an earlier module leaving firstmodule itabs broken and pointing at an invalid address, we retain a
// mapping of all loaded modules, and which itab methods they have patched, so when one is unloaded, we can apply the
// patches from another if required. This assumes that it doesn't matter which CodeModule provides the extra missing
// methods for a given firstmodule's *_type/interface pair (all modules should have their types deduplicated in the same way).
// Since goloader doesn't do any deadcode elimination, a loaded *_type will always include all available methods

func getOtherPatchedMethodsForType(t *_type, currentModule *CodeModule) (otherModule *CodeModule, ifn map[int]struct{}, tfn map[int]struct{}, mtyp map[int]typeOff) {
	modulesLock.Lock()
	defer modulesLock.Unlock()
	for module := range modules {
		if module != currentModule {
			var tfnPatched, ifnPatched, mtypPatched bool
			tfn, tfnPatched = module.patchedTypeMethodsTfn[t]
			ifn, ifnPatched = module.patchedTypeMethodsIfn[t]
			mtyp, mtypPatched = module.patchedTypeMethodsMtyp[t]
			if tfnPatched || ifnPatched || mtypPatched {
				otherModule = module
				return
			}
		}
	}
	return nil, nil, nil, nil
}

func unreachableMethod() {
	panic("goloader patched unreachable method called. linker bug?")
}

// This var gets updated on darwin if the init() in macho_darwin.go gets run successfully
var textIsWriteable = runtime.GOOS != "darwin"

// Since multiple CodeModules might patch the first module we need to make sure to store all indices which have ever been patched
var firstModuleMissingMethods = map[*_type]map[int]struct{}{}

var firstModuleTypemapEntries = map[*_type]typeOff{}
var firstModuleTypemapCounter typeOff = -1

// Similar to runtime.(*itab).init() but replaces method text pointers to start the offset from the specified base address
func (m *itab) adjustMethods(codeBase uintptr, methodIndices map[int]struct{}, writeablePages map[*byte]struct{}) {
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
		for _, j := range sortInts(methodIndices) {
			t := &xmhdr[j]
			tname := typ.nameOff(t.name)
			if typ.typeOff(t.mtyp) == itype && tname.name() == iname {
				pkgPath := tname.pkgPath()
				if pkgPath == "" {
					pkgPath = typ.nameOff(x.pkgpath).name()
				}
				if tname.isExported() || pkgPath == ipkg {
					if m != nil {
						var ifn unsafe.Pointer
						if t.ifn == -1 {
							// -1 is the sentinel value for unreachable code.
							// See cmd/link/internal/ld/data.go:relocsym.
							ifn = unsafe.Pointer(getFunctionPtr(unreachableMethod))
						} else {
							if uintptr(unsafe.Pointer(typ)) >= firstmoduledata.types && uintptr(unsafe.Pointer(typ)) < firstmoduledata.etypes {
								ifn = unsafe.Pointer(firstmoduledata.text + uintptr(t.ifn))
							} else {
								for md := &firstmoduledata; md != nil; md = md.next {
									if uintptr(unsafe.Pointer(typ)) >= md.types && uintptr(unsafe.Pointer(typ)) < md.etypes {
										ifn = unsafe.Pointer(md.text + uintptr(t.ifn))
									}
								}
							}
						}
						page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[k])))
						if _, ok := writeablePages[&page[0]]; !ok {
							err := mprotect.MprotectMakeWritable(page)
							if err != nil {
								panic(err)
							}
							writeablePages[&page[0]] = struct{}{}
						}
						methods[k] = ifn
					}
					continue imethods
				}
			}
		}
	}
}

func (cm *CodeModule) patchTypeMethodOffsets(t *_type, u, prevU *uncommonType, patchedTypeMethodsIfn, patchedTypeMethodsTfn map[*_type]map[int]struct{}, patchedTypeMethodsMtyp map[*_type]map[int]typeOff) (err error) {
	// It's possible that a baked in type in the main module does not have all its methods reachable
	// (i.e. some method offsets will be set to -1 via the linker's reachability analysis) whereas the
	// new type will have them them all.

	// In this case, to avoid fatal "unreachable method called. linker bug?" errors, we need to
	// manipulate the method text and type offsets to make them not -1, and manually partially adjust the
	// firstmodule itabs to rewrite the method addresses to point at the new module text (and remember to clean up afterwards)

	if u != nil && prevU != nil && u != prevU && textIsWriteable {
		methods := u.methods()
		prevMethods := prevU.methods()
		if len(methods) == len(prevMethods) {
			for i := range methods {
				missingIndices, found := firstModuleMissingMethods[t]
				if !found {
					missingIndices = map[int]struct{}{}
					firstModuleMissingMethods[t] = missingIndices
				}
				_, markedMissing := missingIndices[i]

				if methods[i].tfn == -1 || methods[i].ifn == -1 || methods[i].mtyp == -1 || markedMissing {
					missingIndices[i] = struct{}{}

					if prevMethods[i].ifn != -1 {
						if _, ok := patchedTypeMethodsIfn[t]; !ok {
							patchedTypeMethodsIfn[t] = map[int]struct{}{}
						}
						page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].ifn)))
						err = mprotect.MprotectMakeWritable(page)
						if err != nil {
							return fmt.Errorf("failed to make page writeable while patching type %s %p: %w", _name(t.nameOff(t.str)), unsafe.Pointer(&methods[i].ifn), err)
						}
						// The JIT type's ifn would have been offset with respect to the new type's module's text base.
						// Since we're manipulating the firstmodule's type's methods, we need to recompute the offset with respect to the firstmodule's text base
						methodEntry := cm.module.text + uintptr(prevMethods[i].ifn)
						methods[i].ifn = textOff(methodEntry - firstmoduledata.text)
						err = mprotect.MprotectMakeReadOnly(page)
						if err != nil {
							return fmt.Errorf("failed to make page read only while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}
						// Store for later cleanup on Unload()
						patchedTypeMethodsIfn[t][i] = struct{}{}
					}

					if prevMethods[i].tfn != -1 {
						if _, ok := patchedTypeMethodsTfn[t]; !ok {
							patchedTypeMethodsTfn[t] = map[int]struct{}{}
						}
						page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].tfn)))
						err = mprotect.MprotectMakeWritable(page)
						if err != nil {
							return fmt.Errorf("failed to make page writeable while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}

						// The JIT type's tfn would have been offset with respect to the new type's module's text base.
						// Since we're manipulating the firstmodule's type's methods, we need to recompute the offset with respect to the firstmodule's text base
						methodEntry := cm.module.text + uintptr(prevMethods[i].tfn)
						methods[i].tfn = textOff(methodEntry - firstmoduledata.text)
						err = mprotect.MprotectMakeReadOnly(page)
						if err != nil {
							return fmt.Errorf("failed to make page read only while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}
						// Store for later cleanup on Unload()
						patchedTypeMethodsTfn[t][i] = struct{}{}
					}

					if prevMethods[i].mtyp != -1 && methods[i].mtyp < 0 {
						if _, ok := patchedTypeMethodsMtyp[t]; !ok {
							patchedTypeMethodsMtyp[t] = map[int]typeOff{}
						}
						page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].mtyp)))
						err = mprotect.MprotectMakeWritable(page)
						if err != nil {
							return fmt.Errorf("failed to make page writeable while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}

						// The JIT type's mtyp would have been offset with respect to the new type's module's data base.
						// Since the runtime assumes that types and their methods' types are defined in the same module, but we have a situation where the type
						// is in the firstmodule, but method type is in a JIT module, we have to hack around runtime.resolveTypeOff
						// by adding an entry under a negative offset (< -1) to the firstmodule's typemap
						methodType := (*_type)(unsafe.Pointer(cm.module.types + uintptr(prevMethods[i].mtyp)))
						firstModuleTypemapCounter--
						if firstmoduledata.typemap == nil {
							firstmoduledata.typemap = make(map[typeOff]*_type, len(firstmoduledata.typelinks)+1)
							for _, tl := range firstmoduledata.typelinks {
								firstmoduledata.typemap[typeOff(tl)] = (*_type)(unsafe.Pointer(firstmoduledata.types + uintptr(tl)))
							}
							pinnedTypemaps = append(pinnedTypemaps, firstmoduledata.typemap)
						}
						firstmoduledata.typemap[firstModuleTypemapCounter] = methodType
						firstModuleTypemapEntries[methodType] = firstModuleTypemapCounter

						cm.module.typemap[firstModuleTypemapCounter] = methodType // In case we need to resolve this method type with respect to the JIT type (unlikely since it should have been deduped with the firstmodule type?)
						methods[i].mtyp = firstModuleTypemapCounter

						err = mprotect.MprotectMakeReadOnly(page)
						if err != nil {
							return fmt.Errorf("failed to make page read only while patching type %s: %w", _name(t.nameOff(t.str)), err)
						}
						// Store for later cleanup on Unload()
						patchedTypeMethodsMtyp[t][i] = firstModuleTypemapCounter
						markedMissing = true
					}

					if markedMissing {
						if _, ok := patchedTypeMethodsIfn[t]; !ok {
							patchedTypeMethodsIfn[t] = map[int]struct{}{}
						}
						if _, ok := patchedTypeMethodsTfn[t]; !ok {
							patchedTypeMethodsTfn[t] = map[int]struct{}{}
						}
						patchedTypeMethodsIfn[t][i] = struct{}{}
						patchedTypeMethodsTfn[t][i] = struct{}{}
					}

				}
			}
		}
	}
	return nil
}

func firstModuleItabsByType() map[*_type][]*itab {
	firstModule := activeModules()[0]
	result := map[*_type][]*itab{}
	for _, itab := range firstModule.itablinks {
		result[itab._type] = append(result[itab._type], itab)
	}
	return result
}

var sortedInts []int

func sortInts(m map[int]struct{}) []int {
	sortedInts = sortedInts[:0]
	for i := range m {
		sortedInts = append(sortedInts, i)
	}
	sort.Ints(sortedInts)
	return sortedInts
}

func patchTypeMethodTextPtrs(codeBase uintptr, patchedTypeMethodsIfn, patchedTypeMethodsTfn map[*_type]map[int]struct{}) (err error) {
	// Adjust the main module's itabs so that any missing methods now point to new module's text instead of "unreachable code".

	firstModule := activeModules()[0]

	var writeablePages = map[*byte]struct{}{}
	for _, itab := range firstModule.itablinks {
		methodIndicesIfn, ifnPatched := patchedTypeMethodsIfn[itab._type]
		methodIndicesTfn, tfnPatched := patchedTypeMethodsTfn[itab._type]
		if ifnPatched || tfnPatched {
			page := mprotect.GetPage(uintptr(unsafe.Pointer(&itab.fun[0])))
			if _, ok := writeablePages[&page[0]]; !ok {
				err = mprotect.MprotectMakeWritable(page)
				if err != nil {
					return fmt.Errorf("failed to make page writeable while re-initing itab for type %s %p: %w", _name(itab._type.nameOff(itab._type.str)), unsafe.Pointer(&itab.fun[0]), err)
				}
				writeablePages[&page[0]] = struct{}{}
			}
			if ifnPatched {
				itab.adjustMethods(codeBase, methodIndicesIfn, writeablePages)
			}
			if tfnPatched {
				itab.adjustMethods(codeBase, methodIndicesTfn, writeablePages)
			}

		}
	}

	for pageStart := range writeablePages {
		err = mprotect.MprotectMakeReadOnly(mprotect.GetPage(uintptr(unsafe.Pointer(pageStart))))
		if err != nil {
			return fmt.Errorf("failed to make page %p read only while re-initing itab : %w", pageStart, err)
		}
	}
	return nil
}

func (cm *CodeModule) revertPatchedTypeMethods() error {
	firstModuleItabs := firstModuleItabsByType()

	var writeablePages = map[*byte]struct{}{}
	for t, indices := range cm.patchedTypeMethodsIfn {
		u := t.uncommon()
		methods := u.methods()
		// Check if we have any other modules available which provide the same methods
		otherModule, ifnPatchedOther, tfnPatchedOther, _ := getOtherPatchedMethodsForType(t, cm)
		if otherModule != nil {
			for _, itab := range firstModuleItabs[t] {
				itab.adjustMethods(uintptr(otherModule.codeBase), ifnPatchedOther, writeablePages)
				itab.adjustMethods(uintptr(otherModule.codeBase), tfnPatchedOther, writeablePages)
			}
		} else {
			// Reset patched method offsets back to -1
			for _, i := range sortInts(indices) {
				page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].ifn)))
				if _, ok := writeablePages[&page[0]]; !ok {
					err := mprotect.MprotectMakeWritable(page)
					if err != nil {
						return fmt.Errorf("failed to make page writeable while patching type %s %p: %w", _name(t.nameOff(t.str)), unsafe.Pointer(&methods[i].ifn), err)
					}
					writeablePages[&page[0]] = struct{}{}
				}
				methods[i].ifn = -1
			}
			for _, itab := range firstModuleItabs[t] {
				// No other module found, all method offsets should be -1, so codeBase is irrelevant
				itab.adjustMethods(0, indices, writeablePages)
			}

		}
	}
	for t, indices := range cm.patchedTypeMethodsTfn {
		u := t.uncommon()
		methods := u.methods()
		// Check if we have any other modules available which provide the same methods
		otherModule, ifnPatchedOther, tfnPatchedOther, _ := getOtherPatchedMethodsForType(t, cm)
		if otherModule != nil {
			for _, itab := range firstModuleItabs[t] {
				itab.adjustMethods(uintptr(otherModule.codeBase), ifnPatchedOther, writeablePages)
				itab.adjustMethods(uintptr(otherModule.codeBase), tfnPatchedOther, writeablePages)
			}
		} else {
			// Reset patched method offsets back to -1
			for _, i := range sortInts(indices) {
				page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].tfn)))
				if _, ok := writeablePages[&page[0]]; !ok {
					err := mprotect.MprotectMakeWritable(page)
					if err != nil {
						return fmt.Errorf("failed to make page writeable while patching type %s %p: %w", _name(t.nameOff(t.str)), unsafe.Pointer(&methods[i].tfn), err)
					}
					writeablePages[&page[0]] = struct{}{}
				}
				methods[i].tfn = -1
			}
			for _, itab := range firstModuleItabs[t] {
				// No other module found, all method offsets should be -1, so codeBase is irrelevant
				itab.adjustMethods(0, indices, writeablePages)
			}
		}
	}

	for t, indices := range cm.patchedTypeMethodsMtyp {
		u := t.uncommon()
		methods := u.methods()
		// Check if we have any other modules available which provide the same methods
		otherModule, _, _, mtypPatchedOther := getOtherPatchedMethodsForType(t, cm)
		if otherModule != nil {
			for i := range indices {
				if otherTypeOff, ok := mtypPatchedOther[i]; ok {
					page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].mtyp)))
					if _, ok := writeablePages[&page[0]]; !ok {
						err := mprotect.MprotectMakeWritable(page)
						if err != nil {
							return fmt.Errorf("failed to make page writeable while patching type %s %p: %w", _name(t.nameOff(t.str)), unsafe.Pointer(&methods[i].tfn), err)
						}
						writeablePages[&page[0]] = struct{}{}
					}
					delete(firstmoduledata.typemap, methods[i].mtyp)
					methods[i].mtyp = otherTypeOff
				}
			}
		} else {
			// Reset patched method type offsets back to -1, and delete firstmoduledata.typemap entries
			for i := range indices {
				page := mprotect.GetPage(uintptr(unsafe.Pointer(&methods[i].mtyp)))
				if _, ok := writeablePages[&page[0]]; !ok {
					err := mprotect.MprotectMakeWritable(page)
					if err != nil {
						return fmt.Errorf("failed to make page writeable while patching type %s %p: %w", _name(t.nameOff(t.str)), unsafe.Pointer(&methods[i].tfn), err)
					}
					writeablePages[&page[0]] = struct{}{}
				}
				if methods[i].mtyp < -1 {
					delete(firstmoduledata.typemap, methods[i].mtyp)
					methods[i].mtyp = -1
				}
			}
		}
	}

	for pageStart := range writeablePages {
		err := mprotect.MprotectMakeReadOnly(mprotect.GetPage(uintptr(unsafe.Pointer(pageStart))))
		if err != nil {
			return fmt.Errorf("failed to make page %p read only while re-initing itab: %w", pageStart, err)
		}
	}
	return nil
}
