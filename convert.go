//go:build go1.18
// +build go1.18

package goloader

import (
	"fmt"
	"log"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"unsafe"
)

func CanAttemptConversion(oldValue, newValue interface{}) bool {
	oldT := efaceOf(&oldValue)._type
	newT := efaceOf(&newValue)._type
	seen := map[_typePair]struct{}{}
	return typesEqual(oldT, newT, seen)
}

func ConvertTypesAcrossModules(oldModule, newModule *CodeModule, oldValue, newValue interface{}) (res interface{}, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("unexpected panic (this is a bug): %v\n stack trace: %s", v, debug.Stack())
		}
	}()

	// You can't just do a reflect.cvtDirect() across modules if composite types of oldValue/newValue contain an interface.
	// The value stored in the interface could point to the itab from the old module, which might get unloaded
	// So we need to recurse over the entire structure, and find any itabs and replace them with the equivalent from the new module

	oldT := efaceOf(&oldValue)._type
	newT := efaceOf(&newValue)._type
	seen := map[_typePair]struct{}{}
	if !typesEqual(oldT, newT, seen) {
		return nil, fmt.Errorf("old type %T and new type %T are not equal", oldValue, newValue)
	}

	// Need to take data in old value and copy into new value one field at a time, but check that
	// the type is either shared (first module) or translated from the old to the new modules
	newV := Indirect(ValueOf(&newValue)).Elem()
	oldV := Indirect(ValueOf(&oldValue)).Elem()

	cycleDetector := map[uintptr]*Value{}
	typeHash := make(map[uint32][]*_type, len(newModule.module.typelinks))
	buildModuleTypeHash(activeModules()[0], typeHash)
	buildModuleTypeHash(newModule.module, typeHash)

	cvt(oldModule, newModule, Value{oldV}, newV.Type(), nil, cycleDetector, typeHash)

	return oldV.ConvertWithInterface(newV.Type()).Interface(), err
}

func toType(t Type) *_type {
	var x interface{} = t
	return (*_type)(efaceOf(&x).data)
}

type fakeValue struct {
	typ  *_type
	ptr  unsafe.Pointer
	flag uintptr
}

func AsType(_typ *_type) Type {
	var t interface{} = TypeOf("")
	eface := efaceOf(&t)
	eface.data = unsafe.Pointer(_typ)
	return t.(Type)
}

var closureFuncRegex = regexp.MustCompile(`^.*\.func[0-9]+$`)

func cvt(oldModule, newModule *CodeModule, oldValue Value, newType Type, oldValueBeforeElem *Value, cycleDetector map[uintptr]*Value, typeHash map[uint32][]*_type) {
	// By this point we're sure that types are structurally equal, but their *_type addresses might not be

	kind := oldValue.Kind()

	if newType.Kind() != kind {
		panic(fmt.Sprintf("old value's kind (%s) and new type (%s - %s) don't match", kind, newType.String(), newType.Kind()))
	}

	// Non-composite types of equal kind have same underlying type
	if Bool <= kind && kind <= Complex128 || kind == String || kind == UnsafePointer {
		return
	}

	switch kind {
	case Array, Ptr, Slice:
		elemKind := oldValue.Type().Elem().Kind()
		if Bool <= elemKind && elemKind <= Complex128 || elemKind == String || elemKind == UnsafePointer {
			// Shortcut for non-composite types
			return
		}
	}

	// Composite types.
	switch kind {
	case Interface:
		innerVal := oldValue.Elem()
		if innerVal.Kind() == Invalid {
			return
		}
		oldTInner := toType(innerVal.Type())
		oldTOuter := toType(oldValue.Type())
		var newTypeInner *_type
		var newTypeOuter *_type
		types := typeHash[oldTInner.hash]
		for _, _typeNew := range types {
			seen := map[_typePair]struct{}{}
			if oldTInner == _typeNew || typesEqual(oldTInner, _typeNew, seen) {
				newTypeInner = _typeNew
				break
			}
		}

		types = typeHash[oldTOuter.hash]
		for _, _typeNew := range types {
			seen := map[_typePair]struct{}{}
			if oldTOuter == _typeNew || typesEqual(oldTOuter, _typeNew, seen) {
				newTypeOuter = _typeNew
				break
			}
		}

		if newTypeInner == nil {
			oldTAddr := uintptr(unsafe.Pointer(oldTInner))
			if innerVal.Type().PkgPath() == "" || (firstmoduledata.types >= oldTAddr && oldTAddr < firstmoduledata.etypes) {
				newTypeInner = oldTInner
			} else {
				panic(fmt.Sprintf("new module does not contain equivalent type for %s (hash %d)", innerVal.Type(), toType(innerVal.Type()).hash))
			}
		}
		if newTypeOuter == nil {
			oldTAddr := uintptr(unsafe.Pointer(oldTOuter))
			if oldValue.Type().PkgPath() == "" || (firstmoduledata.types >= oldTAddr && oldTAddr < firstmoduledata.etypes) {
				newTypeOuter = oldTOuter
			} else {
				panic(fmt.Sprintf("new module does not contain equivalent type for %s (hash %d)", oldValue.Type(), toType(oldValue.Type()).hash))
			}
		}

		newInnerType := AsType(newTypeInner)
		newOuterType := AsType(newTypeOuter)
		tt := (*interfacetype)(unsafe.Pointer(newTypeOuter))

		if len(tt.mhdr) > 0 {
			iface := (*nonEmptyInterface)(((*fakeValue)(unsafe.Pointer(&oldValue))).ptr)
			if iface.itab == nil {
				// nil value in interface, no further work required
				return
			} else {
				// Need to check whether itab points at old module, and find the equivalent itab in the new module and point to that instead

				var oldItab *itab
				for _, o := range oldModule.module.itablinks {
					if iface.itab == o {
						oldItab = o
						break
					}
				}
				if oldItab != nil {
					var newItab *itab
					for _, n := range newModule.module.itablinks {
						// Need to compare these types carefully
						if oldItab.inter.typ.hash == n.inter.typ.hash && oldItab._type.hash == n._type.hash {
							seen := map[_typePair]struct{}{}
							if typesEqual(&oldItab.inter.typ, &n.inter.typ, seen) && typesEqual(oldItab._type, n._type, seen) {
								newItab = n
								break
							}
						}
					}
					if newItab == nil {
						panic(fmt.Sprintf("could not find equivalent itab for interface %s type %s in new module.", oldValue.Type().String(), oldValue.Elem().Type().String()))
					}
					iface.itab = newItab
				}
			}
		}

		innerValKind := innerVal.Kind()
		if !(Bool <= innerValKind && innerValKind <= Complex128 || innerValKind == String || innerValKind == UnsafePointer) {
			cvt(oldModule, newModule, Value{innerVal}, newInnerType, &oldValue, cycleDetector, typeHash)
		} else {
			if innerVal.CanConvert(newInnerType) {
				newVal := innerVal.Convert(newInnerType)
				if !oldValue.CanSet() {
					if !oldValue.CanAddr() {
						if oldValueBeforeElem != nil && oldValueBeforeElem.Kind() == Interface {
							oldValueBeforeElem.Set(newVal)
						} else {
							panic(fmt.Sprintf("can't set old value of type %s with new value %s (can't address or indirect)", oldValue.Type(), newVal.Type()))
						}
					} else {
						NewAt(newOuterType, unsafe.Pointer(oldValue.UnsafeAddr())).Elem().Set(newVal)
					}
				} else {
					oldValue.Set(newVal)
				}
			} else {
				panic(fmt.Sprintf("can't convert old value of type %s with new value %s", innerVal.Type(), newInnerType))
			}
		}
	case Func:
		oldPtr := oldValue.Pointer()
		if oldPtr != 0 {
			if oldPtr < firstmoduledata.text || oldPtr >= firstmoduledata.etext {
				if oldPtr >= oldModule.module.text && oldPtr < oldModule.module.etext {
					// If the func points at code inside the old module, we need to either find the address of
					// the equivalent func by name, or error if we can't find it
					oldF := runtime.FuncForPC(oldPtr)
					oldFName := oldF.Name()
					if oldFName == "" {
						panic(fmt.Sprintf("old value's function pointer 0x%x does not have a name - cannot convert anonymous functions", oldPtr))
					}
					found := false
					for _, f := range newModule.module.ftab {
						_func := (*_func)(unsafe.Pointer(&(newModule.module.pclntable[f.funcoff])))
						name := getfuncname(_func, newModule.module)
						if name == oldFName {
							entry := getfuncentry(_func, newModule.module.text)
							// This is actually unsafe, because there's no guarantee that the new version
							// of the function has the same signature as the old, and there's no way of accessing
							// the function *_type from just a PC addr, unless the compiler populated a ptab.
							log.Printf("WARNING - converting functions %s by name - no guarantees that signatures will match \n", oldFName)
							newValue := oldValue
							manipulation := (*fakeValue)(unsafe.Pointer(&newValue))
							var funcContainer unsafe.Pointer
							if strings.HasSuffix(oldFName, "-fm") {
								// This is a method, so the data pointer in the value is actually to a closure struct { F uintptr; R *receiver }
								// and the function pointer is to a wrapper func which accepts this struct as its argument
								closure := *(**struct {
									F uintptr
									R unsafe.Pointer
								})(manipulation.ptr)

								// We need to not only set the func entrypoint, but also convert the receiver and set that too
								// TODO - how can we find out the receiver's type in order to convert across modules?
								//  This code might not be safe if the receivers then call other methods?

								// Now check whether the old closure.F is an itab method or a concrete type
								var oldItab *itab
								recvVal := *(*unsafe.Pointer)(closure.R)
								for _, itab := range oldModule.module.itablinks {
									// This deref of the receiver into an 8 byte word is 100% unsafe, but I can't figure out how to find out what the type of R is...
									if unsafe.Pointer(itab.inter) == recvVal {
										oldItab = itab
									}
								}
								if oldItab != nil {
									var newItab *itab
									for _, n := range newModule.module.itablinks {
										// Need to compare these types carefully
										if oldItab.inter.typ.hash == n.inter.typ.hash && oldItab._type.hash == n._type.hash {
											seen := map[_typePair]struct{}{}
											if typesEqual(&oldItab.inter.typ, &n.inter.typ, seen) && typesEqual(oldItab._type, n._type, seen) {
												newItab = n
												break
											}
										}
									}
									if newItab == nil {
										panic(fmt.Sprintf("could not find equivalent itab for interface %s type %s in new module.", oldValue.Type().String(), oldValue.Elem().Type().String()))
									}
									closure.R = unsafe.Pointer(newItab)
								}

								funcContainer = unsafe.Pointer(closure)
								closure.F = entry
							} else if closureFuncRegex.MatchString(oldFName) {
								// This is a closure which is unlikely to be safe since the variables it closes over might be in the old module's memory
								closure := *(**struct {
									F uintptr
									// ... <- variables which are captured by the closure would follow, but we can't know how many they are or what their types are - the best we can do is switch the function implementation and keep the variables the same
								})(manipulation.ptr)
								closure.F = entry
								funcContainer = unsafe.Pointer(closure)
								log.Printf("EVEN BIGGER WARNING - converting anonymous function %s by name - no guarantees that signatures, or the closed over variable sizes, or types will match. This is dangerous! \n", oldFName)
							} else {
								// PC addresses for functions are 2 levels of indirection from a reflect value's word addr,
								// so we allocate addresses on the heap to hold the indirections
								// Normally the RODATA has a pkgname.FuncName·f symbol which stores this - Ideally we would use that instead of the heap

								// TODO - is this definitely safe from GC?
								funcPtr := new(uintptr)
								*funcPtr = entry
								funcContainer = unsafe.Pointer(funcPtr)
							}
							funcPtrContainer := new(unsafe.Pointer)
							*funcPtrContainer = funcContainer
							manipulation.ptr = unsafe.Pointer(funcPtrContainer)
							manipulation.typ = toType(newType)
							if !oldValue.CanSet() {
								if oldValue.CanAddr() {
									oldValue = Value{NewAt(newType, unsafe.Pointer(oldValue.UnsafeAddr())).Elem()}
								} else {
									panic(fmt.Sprintf("can't set old func of type %s with new value 0x%x (can't address or indirect)", oldValue.Type(), entry))
								}
							}
							oldValue.Set(newValue.Value)
							found = true
							break
						}
					}
					if !found {
						panic(fmt.Sprintf("old value's function pointer 0x%x with name %s has no equivalent name in new module - cannot convert", oldPtr, oldFName))
					}
				} else {
					panic(fmt.Sprintf("old value's function pointer 0x%x not in first module (0x%x - 0x%x) nor old module (0x%x - 0x%x) - cannot convert", oldPtr, firstmoduledata.text, firstmoduledata.etext, oldModule.module.text, oldModule.module.etext))
				}
			}
		}
	case Array, Slice:
		for i := 0; i < oldValue.Len(); i++ {
			cvt(oldModule, newModule, Value{oldValue.Index(i)}, newType.Elem(), nil, cycleDetector, typeHash)
		}
	case Map:
		if oldValue.Len() == 0 {
			return
		}
		keyType := oldValue.Type().Key()
		valType := oldValue.Type().Elem()
		mvKind := valType.Kind()
		mkKind := keyType.Kind()
		if !(Bool <= mvKind && mvKind <= Complex128 || mvKind == String || mvKind == UnsafePointer) ||
			!(Bool <= mkKind && mkKind <= Complex128 || mkKind == String || mkKind == UnsafePointer) {
			// Need to recreate map entirely since they aren't mutable
			newMap := MakeMapWithSize(oldValue.Type(), oldValue.Len())
			mapKeys := oldValue.MapKeys()
			for _, mapKey := range mapKeys {
				mapValue := oldValue.MapIndex(mapKey)
				var nk Value
				if mkKind == Ptr {
					nk = Value{New(keyType.Elem())}
				} else {
					nk = Value{Indirect(New(keyType))}
				}
				var nv Value
				if mvKind == Ptr {
					nv = Value{New(valType.Elem())}
				} else {
					nv = Value{Indirect(New(valType))}
				}
				if mkKind == Ptr {
					nk.Elem().Set(mapKey.Elem())
				} else {
					nk.Set(mapKey)
				}
				if mvKind == Ptr {
					nv.Elem().Set(mapValue.Elem())
				} else {
					nv.Set(mapValue)
				}

				cvt(oldModule, newModule, nv, newType.Elem(), &oldValue, cycleDetector, typeHash)
				cvt(oldModule, newModule, nk, newType.Key(), &oldValue, cycleDetector, typeHash)
				newMap.SetMapIndex(nk.Value, nv.Value)
			}
			if !oldValue.CanSet() {
				if oldValue.CanAddr() {
					oldValue = Value{NewAt(oldValue.Type(), unsafe.Pointer(oldValue.UnsafeAddr())).Elem()}
				} else {
					panic(fmt.Sprintf("can't set old map of type %s with new value (can't address or indirect)", oldValue.Type()))
				}
			}
			oldValue.Set(newMap)
		}
	case Ptr:
		if !oldValue.IsNil() {
			up := oldValue.Pointer()
			if _, cyclic := cycleDetector[up]; cyclic {
				return
			} else {
				cycleDetector[up] = &oldValue
				cvt(oldModule, newModule, Value{oldValue.Elem()}, newType.Elem(), &oldValue, cycleDetector, typeHash)
			}
		}
	case Struct:
		for i := 0; i < oldValue.NumField(); i++ {
			field := oldValue.Field(i)
			fieldKind := field.Kind()
			if !(Bool <= fieldKind && fieldKind <= Complex128 || fieldKind == String || fieldKind == UnsafePointer) {
				cvt(oldModule, newModule, Value{field}, newType.Field(i).Type, nil, cycleDetector, typeHash)
			}
		}
	}
}
