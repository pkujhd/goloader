//go:build darwin
// +build darwin

package jit

import (
	"reflect"
	"syscall"
)

func bakeInPlatform() {
	_ = reflect.TypeOf(reflect.ValueOf(syscall.Setuid))
}
