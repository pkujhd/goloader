//go:build go1.8 && !go1.9
// +build go1.8,!go1.9

package goloader

import "github.com/pkujhd/goloader/obj"

type inlinedCall struct{}

func (linker *Linker) addInlineTree(_func *_func, objsym *obj.ObjSymbol) (err error) {
	return nil
}
