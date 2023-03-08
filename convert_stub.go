//go:build !go1.18
// +build !go1.18

package goloader

import "fmt"

func CanAttemptConversion(oldValue, newValue interface{}) bool {
	return false
}

func ConvertTypesAcrossModules(oldModule, newModule *CodeModule, oldValue, newValue interface{}) (res interface{}, err error) {
	return nil, fmt.Errorf("not supported in this older Go version yet - requires backport")
}
