//go:build go1.8 && !go1.12
// +build go1.8,!go1.12

package dataindex

// This file defines the IDs for PCDATA and FUNCDATA instructions
// in Go binaries.
//
// These must agree with ../../../runtime/funcdata.h and
// ../../../runtime/symtab.go.
const (
	PCDATA_StackMapIndex = 0
	PCDATA_InlTreeIndex  = 1

	FUNCDATA_ArgsPointerMaps   = 0
	FUNCDATA_LocalsPointerMaps = 1
	FUNCDATA_InlTree           = 2
	
	ArgsSizeUnknown = -0x80000000
)
