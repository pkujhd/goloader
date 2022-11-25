#include "textflag.h"

#define PosInf 0x7FF0000000000000
#define NaN    0x7FF8000000000001
#define NegInf 0xFFF0000000000000

// func ·archMax(x, y float64) float64
TEXT ·archMax(SB),NOSPLIT,$0
	// +Inf special cases
	MOVD	$PosInf, R0
	MOVD	x+0(FP), R1
	CMP	R0, R1
	BEQ	isPosInf
	MOVD	y+8(FP), R2
	CMP	R0, R2
	BEQ	isPosInf
	// normal case
	FMOVD	R1, F0
	FMOVD	R2, F1
	FMAXD	F0, F1, F0
	FMOVD	F0, ret+16(FP)
	RET
isPosInf: // return +Inf
	MOVD	R0, ret+16(FP)
	RET
