//go:build go1.19
// +build go1.19

package jit

import (
	"reflect"
	"runtime/debug"
)

func bakeInVersion() {
	_ = reflect.ValueOf(debug.SetMemoryLimit)
}
