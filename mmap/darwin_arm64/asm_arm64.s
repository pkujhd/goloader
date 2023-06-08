#include "textflag.h"

TEXT ·pthread_jit_write_protect_np_trampoline(SB),NOSPLIT,$0
	MOVD	R0, R19
	MOVW	0(R19), R0	// arg 1 enable
	BL	libpthread_pthread_jit_write_protect_np(SB)
	RET

TEXT ·sys_icache_invalidate_trampoline(SB),NOSPLIT,$0
	MOVD	R0, R19
	MOVD	0(R19), R0	// arg 1 ptr
	MOVD	8(R19), R1	// arg 2 size
	BL	libkern_sys_icache_invalidate(SB)
	RET
