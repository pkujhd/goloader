#include "textflag.h"
TEXT ·libc_mmap_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_mmap(SB)
TEXT ·libc_munmap_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_munmap(SB)
