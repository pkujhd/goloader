//go:build go1.20 && !go1.25
// +build go1.20,!go1.25

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
	PCDATA_ArgLiveIndex  = 3

	FUNCDATA_ArgsPointerMaps    = 0
	FUNCDATA_LocalsPointerMaps  = 1
	FUNCDATA_StackObjects       = 2
	FUNCDATA_InlTree            = 3
	FUNCDATA_OpenCodedDeferInfo = 4
	FUNCDATA_ArgInfo            = 5
	FUNCDATA_ArgLiveInfo        = 6
	FUNCDATA_WrapInfo           = 7

	// ArgsSizeUnknown is set in Func.argsize to mark all functions
	// whose argument size is unknown (C vararg functions, and
	// assembly code without an explicit specification).
	// This value is generated by the compiler, assembler, or linker.
	ArgsSizeUnknown = -0x80000000
)

// Special PCDATA values.
const (
	// PCDATA_UnsafePoint values.
	PCDATA_UnsafePointSafe   = -1 // Safe for async preemption
	PCDATA_UnsafePointUnsafe = -2 // Unsafe for async preemption

	// PCDATA_Restart1(2) apply on a sequence of instructions, within
	// which if an async preemption happens, we should back off the PC
	// to the start of the sequence when resuming.
	// We need two so we can distinguish the start/end of the sequence
	// in case that two sequences are next to each other.
	PCDATA_Restart1 = -3
	PCDATA_Restart2 = -4

	// Like PCDATA_Restart1, but back to function entry if async preempted.
	PCDATA_RestartAtEntry = -5
)
