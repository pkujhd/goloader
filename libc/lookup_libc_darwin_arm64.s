#include "textflag.h"

TEXT ·libc_dlopen_trampoline(SB),NOSPLIT,$0
	MOVD	R0, R19
	MOVD	0(R19), R0	// arg 1 soPath
	MOVD	8(R19), R1	// arg 2 flags
	BL	libc_dlopen(SB)
	MOVD	$0, R1
	MOVD	$-1, R2
	CMP	R0, R2
	BNE	ok
	BL	libc_error(SB)
	MOVW	(R0), R1
	MOVD	$0, R0
ok:
	MOVD	R0, 16(R19) // ret 1 p
	MOVD	R1, 24(R19)	// ret 2 err
	RET

TEXT ·libc_dlsym_trampoline(SB),NOSPLIT,$0
	MOVD	R0, R19
	MOVD	0(R19), R0	// arg 1 handle
	MOVD	8(R19), R1	// arg 2 symName
	BL	libc_dlsym(SB)
	MOVD	$0, R1
	MOVD	$-1, R2
	CMP	R0, R2
	BNE	ok
	BL	libc_error(SB)
	MOVW	(R0), R1
	MOVD	$0, R0
ok:
	MOVD	R0, 16(R19) // ret 1 p
	RET
