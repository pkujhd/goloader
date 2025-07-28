//go:build go1.21 && !go1.26
// +build go1.21,!go1.26

package link

import (
	"github.com/pkujhd/goloader/objabi/reloctype"
	"reflect"
	"regexp"
	"strings"
	"unsafe"

	"github.com/pkujhd/goloader/obj"
)

type initTask struct {
	state uint32 // 0 = uninitialized, 1 = in progress, 2 = done
	nfns  uint32
	// followed by nfns pcs, uintptr sized, one per init function to run
}

const (
	_InitTaskSuffix = "..inittask"
	_InitFunction   = ".init"

	_InitTaskSize = int(unsafe.Sizeof(initTask{}))
)

func getInitFuncName(packageName string) string {
	return obj.PathToPrefix(packageName) + _InitTaskSuffix
}

func isInitTaskName(name string) bool {
	return strings.HasSuffix(name, _InitTaskSuffix)
}

func isInitFuncName(funcName string) bool {
	if strings.HasSuffix(funcName, _InitFunction) {
		return true
	}
	exp, _ := regexp.Compile(`.init.\s+`)
	return exp.MatchString(funcName)
}

func isNeedInitTaskInPlugin(name string) bool {
	return isInitTaskName(name)
}

func fakeInit() {}

//go:linkname doInit1 runtime.doInit1
func doInit1(t unsafe.Pointer) // t should be a *runtime.initTask

func doInit(ptr, fakeInitPtr uintptr) {
	p := adduintptr(ptr, 0)
	task := *(*initTask)(p)
	if task.nfns != 0 {
		if ptr < firstmoduledata.text || ptr > firstmoduledata.etext {
			// if a xxx.init.x in runtime environment, replace it with fake init
			for i := uint32(0); i < task.nfns; i++ {
				funcAddr := ptr + uintptr(_InitTaskSize) + uintptr(i*PtrSize)
				addr := *(*uintptr)(unsafe.Pointer(funcAddr))
				if addr >= firstmoduledata.text && addr <= firstmoduledata.etext {
					*(*uintptr)(unsafe.Pointer(funcAddr)) = fakeInitPtr
				}
			}
		}
		doInit1(p)
	}
}

func (linker *Linker) isDependent(pkgPath string) bool {
	for _, pkg := range linker.Packages {
		for _, imported := range pkg.ImportPkgs {
			if imported == pkgPath {
				return false
			}
		}
	}
	return true
}

func (linker *Linker) getEntryPackage() *obj.Pkg {
	for _, pkg := range linker.Packages {
		if linker.isDependent(pkg.PkgPath) {
			return pkg
		}
	}
	return nil
}

func (linker *Linker) importedPackages() []string {
	if len(linker.Packages) == 0 {
		return nil
	}
	mainPkg := linker.getEntryPackage()
	if mainPkg == nil {
		return nil
	}
	iPackages := make([]string, 0)
	seen := make(map[string]bool)
	linker.sortedImportedPackages(mainPkg.PkgPath, &iPackages, seen)
	return iPackages
}

func (linker *Linker) sortedImportedPackages(targetPkg string, iPackages *[]string, seen map[string]bool) {
	if _, ok := seen[targetPkg]; !ok {
		seen[targetPkg] = true
		if _, ok := linker.Packages[targetPkg]; ok {
			for _, imported := range linker.Packages[targetPkg].ImportPkgs {
				linker.sortedImportedPackages(imported, iPackages, seen)
				if _, ok := linker.Packages[imported]; ok {
					nImporteds := linker.Packages[imported].ImportPkgs
					for _, nImported := range nImporteds {
						if _, ok := seen[nImported]; !ok {
							*iPackages = append(*iPackages, nImported)
						}
					}
				}
			}
		}
		*iPackages = append(*iPackages, targetPkg)
	}
}

func (linker *Linker) doInitialize(symPtr, symbolMap map[string]uintptr) error {
	fakeInitPtr := reflect.ValueOf(fakeInit).Pointer()
	for _, pkgPath := range linker.importedPackages() {
		if ptr, ok := symbolMap[getInitFuncName(pkgPath)]; ok {
			doInit(ptr, fakeInitPtr)
		}
	}
	return nil
}

//go:inline
func isInSymPtrMap(symPtr map[string]uintptr, name string) bool {
	_, ok := symPtr[name]
	return ok
}

func isRelocSymbolsExist(symbolMap map[string]*obj.ObjSymbol, symbolName string, symPtr map[string]uintptr) bool {
	symbol := symbolMap[symbolName]
	retValue := true
	for _, reloc := range symbol.Reloc {
		name := reloc.SymName
		if isInitTaskName(name) {
		} else if isInitFuncName(name) {
			if isInSymPtrMap(symPtr, name) {
				if !isRelocSymbolsExist(symbolMap, name, symPtr) {
					delete(symPtr, name)
					retValue = false
				}
			} else {
				retValue = false
			}
		} else {
			if strings.HasSuffix(name, GOTPCRELSuffix) {
				name = strings.TrimSuffix(name, GOTPCRELSuffix)
			}
			if !isInSymPtrMap(symPtr, name) && !isStringTypeName(name) &&
				!isTypeName(name) && !isItabName(name) && name != EmptyString &&
				reloc.Type != reloctype.R_CALL|reloctype.R_WEAK {
				retValue = false
			}
		}
	}

	if !retValue {
		if _, ok := symPtr[symbolName]; ok {
			delete(symPtr, symbolName)
		}
	}
	return retValue
}

func isCompleteInitialization(linker *Linker, name string, symPtr map[string]uintptr) bool {
	retValue := true
	if _, ok := linker.ObjSymbolMap[name]; ok && !isRelocSymbolsExist(linker.ObjSymbolMap, name, symPtr) {
		retValue = false
	}
	return retValue
}
