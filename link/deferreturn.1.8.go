//go:build go1.8 && !go1.14
// +build go1.8,!go1.14

package link

func (linker *Linker) addDeferReturn(_func *_func, module *moduledata) (err error) {
	return nil
}
