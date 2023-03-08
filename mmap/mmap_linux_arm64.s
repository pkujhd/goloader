#include "textflag.h"

// func read_CTR_ELO_Register() uint32
TEXT ·read_CTR_ELO_Register(SB),NOSPLIT|NOFRAME,$0
  MRS  CTR_EL0, R0
  MOVW R0, ret+0(FP)
  RET

// clearInstructionCacheLine(addr uintptr)
TEXT ·clearInstructionCacheLine(SB),NOSPLIT|NOFRAME,$0
	MOVD	start+0(FP), R0
  // IC R0, IVAU
  WORD $0xd50b7520
  RET

// clearDataCacheLine(addr uintptr)
TEXT ·clearDataCacheLine(SB),NOSPLIT|NOFRAME,$0
	MOVD	start+0(FP), R0
  // DC R0, CVAU
  WORD $0xd50b7b20
  RET

TEXT ·dataSyncBarrierInnerShareableDomain(SB),NOSPLIT|NOFRAME,$0
  // DSB ISH
  WORD $0xd5033b9f
  RET

TEXT ·instructionSyncBarrier(SB),NOSPLIT|NOFRAME,$0
  // ISB
  WORD $0xd5033fdf
  RET
