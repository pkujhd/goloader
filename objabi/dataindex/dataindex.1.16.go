//go:build go1.16 && !go1.18
// +build go1.16,!go1.18

package dataindex

// This file defines the IDs for PCDATA and FUNCDATA instructions
// in Go binaries.
//
// These must agree with ../../../runtime/funcdata.h and
// ../../../runtime/symtab.go.
const (
	PCDATA_UnsafePoint   = 0
	PCDATA_StackMapIndex = 1
	PCDATA_InlTreeIndex  = 2

	FUNCDATA_ArgsPointerMaps    = 0
	FUNCDATA_LocalsPointerMaps  = 1
	FUNCDATA_StackObjects       = 2
	FUNCDATA_InlTree            = 3
	FUNCDATA_OpenCodedDeferInfo = 4

	ArgsSizeUnknown = -0x80000000
)
