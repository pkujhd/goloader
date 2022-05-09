//go:build go1.8 && !go1.12
// +build go1.8,!go1.12

package stackobject

import "github.com/pkujhd/goloader/obj"

func AddStackObject(funcname string, symMap map[string]*obj.Sym, symbolMap map[string]uintptr, noptrdata uintptr) (err error) {
	return nil
}
