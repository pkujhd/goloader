//go:build go1.12 && !go1.13
// +build go1.12,!go1.13

package dataindex

// This file defines the IDs for PCDATA and FUNCDATA instructions
// in Go binaries.
//
// These must agree with ../../../runtime/funcdata.h and
// ../../../runtime/symtab.go.
const (
	PCDATA_StackMapIndex       = 0
	PCDATA_InlTreeIndex        = 1
	PCDATA_RegMapIndex         = 2
	FUNCDATA_ArgsPointerMaps   = 0
	FUNCDATA_LocalsPointerMaps = 1
	FUNCDATA_InlTree           = 2
	FUNCDATA_RegPointerMaps    = 3
	FUNCDATA_StackObjects      = 4
	ArgsSizeUnknown            = -0x80000000
)
