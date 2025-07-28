package goloader

import (
	"github.com/pkujhd/goloader/link"
	"github.com/pkujhd/goloader/mmap"
	"io"
)

type Linker link.Linker
type CodeModule link.CodeModule

func ReadObj(file, pkgPath string) (*Linker, error) {
	linker, err := link.ReadObj(file, pkgPath)
	return (*Linker)(linker), err
}

func ReadObjs(files []string, pkgPaths []string) (*Linker, error) {
	linker, err := link.ReadObjs(files, pkgPaths)
	return (*Linker)(linker), err
}

func Load(linker *Linker, symPtr map[string]uintptr) (codeModule *CodeModule, err error) {
	module, err := link.Load((*link.Linker)(linker), symPtr)
	return (*CodeModule)(module), err
}

func (codeModule *CodeModule) Unload() {
	(*link.CodeModule)(codeModule).Unload()
}

func UnresolvedSymbols(linker *Linker, symPtr map[string]uintptr) []string {
	return link.UnresolvedSymbols((*link.Linker)(linker), symPtr)
}

func ReadDependPackages(linker *Linker, files, pkgPaths []string, symbolNames []string, symPtr map[string]uintptr) error {
	return link.ReadDependPackages((*link.Linker)(linker), files, pkgPaths, symbolNames, symPtr)
}

func Mmap(size int) ([]byte, error) {
	return mmap.Mmap(size)
}

func Munmap(b []byte) (err error) {
	return mmap.Munmap(b)
}

func Parse(file, pkgPath string) ([]string, error) {
	return link.Parse(file, pkgPath)
}

func RegSymbolWithSo(symPtr map[string]uintptr, path string) error {
	return link.RegSymbolWithSo(symPtr, path)
}

func RegSymbol(symPtr map[string]uintptr) error {
	return link.RegSymbol(symPtr)
}

func RegSymbolWithPath(symPtr map[string]uintptr, path string) error {
	return link.RegSymbolWithPath(symPtr, path)
}

func RegTypes(symPtr map[string]uintptr, interfaces ...interface{}) {
	link.RegTypes(symPtr, interfaces)
}

func Serialize(linker *Linker, writer io.Writer) error {
	return link.Serialize((*link.Linker)(linker), writer)
}

func UnSerialize(reader io.Reader) (*Linker, error) {
	linker, err := link.UnSerialize(reader)
	return (*Linker)(linker), err
}
