//go:build go1.18 && !go1.19
// +build go1.18,!go1.19

package goloader

import "github.com/pkujhd/goloader/reflectlite/reflectlite1.18"

const (
	Invalid       = reflectlite.Invalid
	Bool          = reflectlite.Bool
	Int           = reflectlite.Int
	Int8          = reflectlite.Int8
	Int16         = reflectlite.Int16
	Int32         = reflectlite.Int32
	Int64         = reflectlite.Int64
	Uint          = reflectlite.Uint
	Uint8         = reflectlite.Uint8
	Uint16        = reflectlite.Uint16
	Uint32        = reflectlite.Uint32
	Uint64        = reflectlite.Uint64
	Uintptr       = reflectlite.Uintptr
	Float32       = reflectlite.Float32
	Float64       = reflectlite.Float64
	Complex64     = reflectlite.Complex64
	Complex128    = reflectlite.Complex128
	Array         = reflectlite.Array
	Chan          = reflectlite.Chan
	Func          = reflectlite.Func
	Interface     = reflectlite.Interface
	Map           = reflectlite.Map
	Pointer       = reflectlite.Pointer
	Slice         = reflectlite.Slice
	String        = reflectlite.String
	Struct        = reflectlite.Struct
	UnsafePointer = reflectlite.UnsafePointer
	Ptr           = reflectlite.Ptr
)

var Indirect = reflectlite.Indirect
var ValueOf = reflectlite.ValueOf
var TypeOf = reflectlite.TypeOf
var New = reflectlite.New
var NewAt = reflectlite.NewAt
var MakeMapWithSize = reflectlite.MakeMapWithSize

type Value struct {
	reflectlite.Value
}

type Type reflectlite.Type
