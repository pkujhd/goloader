//go:build go1.13 && !go1.16
// +build go1.13,!go1.16

package dataindex

// This file defines the IDs for PCDATA and FUNCDATA instructions
// in Go binaries.
//
// These must agree with ../../../runtime/funcdata.h and
// ../../../runtime/symtab.go.
const (
	PCDATA_RegMapIndex   = 0
	PCDATA_StackMapIndex = 1
	PCDATA_InlTreeIndex  = 2

	FUNCDATA_ArgsPointerMaps    = 0
	FUNCDATA_LocalsPointerMaps  = 1
	FUNCDATA_RegPointerMaps     = 2
	FUNCDATA_StackObjects       = 3
	FUNCDATA_InlTree            = 4
	FUNCDATA_OpenCodedDeferInfo = 5
	ArgsSizeUnknown             = -0x80000000
)
