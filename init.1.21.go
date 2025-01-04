//go:build go1.21 && !go1.25
// +build go1.21,!go1.25

package goloader

import (
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
)

func getInitFuncName(packagename string) string {
	return obj.PathToPrefix(packagename) + _InitTaskSuffix
}

//go:linkname doInit1 runtime.doInit1
func doInit1(t unsafe.Pointer) // t should be a *runtime.initTask

func doInit(ptr uintptr) {
	p := adduintptr(ptr, 0)
	task := *(*initTask)(p)
	if task.nfns != 0 {
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

func (linker *Linker) getEnteyPackage() *obj.Pkg {
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
	mainPkg := linker.getEnteyPackage()
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
	for _, pkgPath := range linker.importedPackages() {
		if ptr, ok := symbolMap[getInitFuncName(pkgPath)]; ok {
			doInit(ptr)
		}
	}
	return nil
}
