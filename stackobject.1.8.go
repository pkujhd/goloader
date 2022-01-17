//go:build go1.8 && !go1.12
// +build go1.8,!go1.12

package goloader

func (linker *Linker) addStackObject(funcname string, symbolMap map[string]uintptr, module *moduledata) (err error) {
	return nil
}
