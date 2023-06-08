#include "textflag.h"

TEXT ·libc_dlopen_trampoline(SB),NOSPLIT,$0
	PUSHQ	BP	// make a frame; keep stack aligned
	MOVQ	SP, BP
	MOVQ	DI, BX
	MOVQ	0(BX), DI	// arg 1 soPath
	MOVQ	8(BX), SI	// arg 2 flags
	CALL	libc_dlopen(SB)
	XORL	DX, DX
	CMPQ	AX, $-1
	JNE	ok
	CALL	libc_error(SB)
	MOVLQSX	(AX), DX		// errno
	XORL	AX, AX
ok:
	MOVQ	AX, 16(BX)
	MOVQ	DX, 24(BX)
	POPQ	BP
	RET

TEXT ·libc_dlsym_trampoline(SB),NOSPLIT,$0
	PUSHQ	BP	// make a frame; keep stack aligned
	MOVQ	SP, BP
	MOVQ	DI, BX
	MOVQ	0(BX), DI	// arg 1 handle
	MOVQ	8(BX), SI	// arg 2 symName
	CALL	libc_dlsym(SB)
	XORL	DX, DX
	CMPQ	AX, $-1
	JNE	ok
	CALL	libc_error(SB)
	MOVLQSX	(AX), DX		// errno
	XORL	AX, AX
ok:
	MOVQ	AX, 16(BX)
	POPQ	BP
	RET
