package goloader

import (
	"cmd/objfile/objabi"
	"encoding/binary"
	"fmt"
	"github.com/eh-steve/goloader/obj"
	"github.com/eh-steve/goloader/objabi/reloctype"
	"github.com/eh-steve/goloader/objabi/symkind"
	"github.com/eh-steve/goloader/objabi/tls"
	"strings"
	"unsafe"
)

const (
	maxExtraInstructionBytesADRP            = 16
	maxExtraInstructionBytesADRPLDST        = 20
	maxExtraInstructionBytesCALLARM64       = 16
	maxExtraInstructionBytesPCRELxLEAQ      = 8
	maxExtraInstructionBytesPCRELxMOVShort  = 12
	maxExtraInstructionBytesPCRELxMOVNear   = 15
	maxExtraInstructionBytesPCRELxCMPLShort = 18
	maxExtraInstructionBytesPCRELxCMPLNear  = 21
	maxExtraInstructionBytesPCRELxCALLShort = 16
	maxExtraInstructionBytesPCRELxCALLNear  = 19
	maxExtraInstructionBytesPCRELxJMP       = 14
	maxExtraInstructionBytesCALL            = 14
)

func (linker *Linker) relocateADRP(mCode []byte, loc obj.Reloc, segment *segment, symAddr uintptr) (err error) {
	byteorder := linker.Arch.ByteOrder
	signedOffset := int64(symAddr) + int64(loc.Add) - ((int64(segment.codeBase) + int64(loc.Offset)) &^ 0xFFF)
	if oldMcode, ok := linker.appliedADRPRelocs[&mCode[0]]; !ok {
		linker.appliedADRPRelocs[&mCode[0]] = make([]byte, 8)
		copy(linker.appliedADRPRelocs[&mCode[0]], mCode)
	} else {
		copy(mCode, oldMcode)
	}
	epilogueOffset := loc.EpilogueOffset
	copy(segment.codeByte[epilogueOffset:epilogueOffset+loc.EpilogueSize], make([]byte, loc.EpilogueSize))
	// R_ADDRARM64 relocs include 2x 32 bit instructions, one ADRP, and one ADD/LDR/STR - both contain the destination register in the lowest 5 bits
	if signedOffset > 1<<32 || signedOffset < -1<<32 {
		if loc.EpilogueSize == 0 {
			return fmt.Errorf("relocation epilogue not available but got a >32-bit ADRP reloc with offset %d: %s", signedOffset, loc.Sym.Name)
		}
		// Too far to fit inside an ADRP+ADD, do a jump to some extra code we add at the end big enough to fit any 64 bit address
		symAddr += uintptr(loc.Add)
		adrp := byteorder.Uint32(mCode)
		bcode := byteorder.Uint32(arm64Bcode) // Unconditional branch
		bcode |= ((uint32(epilogueOffset) - uint32(loc.Offset)) >> 2) & 0x01FFFFFF
		if epilogueOffset-loc.Offset < 0 {
			bcode |= 0x02000000 // 26th bit is sign bit
		}
		byteorder.PutUint32(mCode, bcode) // The second ADD/LD/ST instruction in the ADRP reloc will be bypassed as we return from the jump after it

		ldrCode8Bytes := uint32(0x58000040)  // LDR PC+8
		ldrCode12Bytes := uint32(0x58000060) // LDR PC+12
		ldrCode8Bytes |= adrp & 0x1F         // Set the register
		ldrCode12Bytes |= adrp & 0x1F        // Set the register

		if loc.Type == reloctype.R_ADDRARM64 {
			byteorder.PutUint32(segment.codeByte[epilogueOffset:], ldrCode8Bytes)
			epilogueOffset += Uint32Size
		} else {
			// must be LDR/STR reloc - the entire 64 bit address will be loaded in the register specified in the ADRP instruction,
			// so should be able to just append the LDR or STR immediately after
			byteorder.PutUint32(segment.codeByte[epilogueOffset:], ldrCode12Bytes)
			epilogueOffset += Uint32Size
			ldOrSt := byteorder.Uint32(mCode[4:])
			byteorder.PutUint32(segment.codeByte[epilogueOffset:], ldOrSt)
			epilogueOffset += Uint32Size
		}

		bcode = byteorder.Uint32(arm64Bcode)
		bcode |= ((uint32(loc.Offset) - uint32(epilogueOffset) + PtrSize) >> 2) & 0x01FFFFFF
		if loc.Offset-epilogueOffset+PtrSize < 0 {
			bcode |= 0x02000000
		}
		byteorder.PutUint32(segment.codeByte[epilogueOffset:], bcode)
		epilogueOffset += Uint32Size

		putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(symAddr))
	} else {
		// Bit layout of ADRP instruction is:

		// 31  30  29  28  27  26  25  24  23  22  21  20  19  18  17  16  15  14  13  11  10  09  08  07  06  05  04  03  02  01  00
		// op  [imlo]   1   0   0   0   0  [<----------------------------- imm hi ----------------------------->]  [  dst register  ]

		// Bit layout of ADD instruction (64-bit) is:

		// 31  30  29  28  27  26  25  24  23  22  21  20  19  18  17  16  15  14  13  11  10  09  08  07  06  05  04  03  02  01  00
		//  1   0   0   1   0   0   0   1   0   0  [<--------------- imm12 ---------------->]  [  src register  ]  [  dst register  ]
		// sf <- 64 bit                        sh <- whether to left shift imm12 by 12 bits

		immLow := uint32((uint64(signedOffset)>>12)&3) << 29
		immHigh := uint32((uint64(signedOffset)>>12>>2)&0x7FFFF) << 5
		adrp := byteorder.Uint32(mCode[0:4])
		adrp |= immLow | immHigh
		addOrLdOrSt := byteorder.Uint32(mCode[4:8])
		switch loc.Type {
		case reloctype.R_ADDRARM64, reloctype.R_ARM64_PCREL_LDST8:
			addOrLdOrSt |= uint32(uint64(signedOffset)&0xFFF) << 10
		case reloctype.R_ARM64_PCREL_LDST16:
			if signedOffset&0x1 != 0 {
				err = fmt.Errorf("offset for 16-bit load/store has unaligned value %d", signedOffset&0xFFF)
			}
			addOrLdOrSt |= (uint32(signedOffset&0xFFF) >> 1) << 10
		case reloctype.R_ARM64_PCREL_LDST32:
			if signedOffset&0x3 != 0 {
				err = fmt.Errorf("offset for 32-bit load/store has unaligned value %d", signedOffset&0xFFF)
			}
			addOrLdOrSt |= (uint32(signedOffset&0xFFF) >> 2) << 10
		case reloctype.R_ARM64_PCREL_LDST64:
			if signedOffset&0x7 != 0 {
				err = fmt.Errorf("offset for 64-bit load/store has unaligned value %d", signedOffset&0xFFF)
			}
			addOrLdOrSt |= (uint32(signedOffset&0xFFF) >> 3) << 10
		}
		byteorder.PutUint32(mCode, adrp)
		byteorder.PutUint32(mCode[4:], addOrLdOrSt)
	}
	return err
}

func (linker *Linker) relocateCALL(addr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) error {
	byteorder := linker.Arch.ByteOrder
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	epilogueOffset := loc.EpilogueOffset
	copy(segment.codeByte[epilogueOffset:epilogueOffset+loc.EpilogueSize], make([]byte, loc.EpilogueSize))

	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		if loc.EpilogueSize == 0 {
			return fmt.Errorf("relocation epilogue not available but got a >32-bit CALL reloc (x86 code: %x) with offset %d: %s", relocByte[loc.Offset-2:loc.Offset+loc.Size], offset, loc.Sym.Name)
		}
		offset = (segment.codeBase + epilogueOffset) - (addrBase + loc.Offset + loc.Size)
		copy(segment.codeByte[epilogueOffset:], x86amd64JMPLcode)
		epilogueOffset += len(x86amd64JMPLcode)
		putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(addr)+uint64(loc.Add))
	}
	byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
	return nil
}

func (linker *Linker) relocatePCREL(addr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) (err error) {
	byteorder := linker.Arch.ByteOrder
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	epilogueOffset := loc.EpilogueOffset
	if oldMcode, ok := linker.appliedPCRelRelocs[&relocByte[loc.Offset]]; !ok {
		linker.appliedPCRelRelocs[&relocByte[loc.Offset]] = make([]byte, loc.Size+2)
		copy(linker.appliedPCRelRelocs[&relocByte[loc.Offset]], relocByte[loc.Offset-2:])
	} else {
		copy(relocByte[loc.Offset-2:], oldMcode)
	}
	copy(segment.codeByte[epilogueOffset:epilogueOffset+loc.EpilogueSize], make([]byte, loc.EpilogueSize))
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		if loc.EpilogueSize == 0 {
			return fmt.Errorf("relocation epilogue not available but got a >32-bit PCREL reloc (x86 code: %x) with offset %d: %s", relocByte[loc.Offset-2:loc.Offset+loc.Size], offset, loc.Sym.Name)
		}
		relocToEpilogueOffset := (segment.codeBase + epilogueOffset) - (addrBase + loc.Offset + loc.Size)
		bytes := relocByte[loc.Offset-2:]
		opcode := relocByte[loc.Offset-2]
		register := ZeroByte

		if opcode == x86amd64LEAcode {
			bytes[0] = x86amd64MOVcode
		} else if opcode == x86amd64MOVcode && loc.Size >= Uint32Size {
			register = ((relocByte[loc.Offset-1] >> 3) & 0x7) | 0xb8
			copy(bytes, append(x86amd64JMPNearCode, x86amd64NOPcode))
		} else if opcode == x86amd64CMPLcode && loc.Size >= Uint32Size {
			copy(bytes, append(x86amd64JMPNearCode, x86amd64NOPcode))
		} else if (bytes[1] == x86amd64CALLcode) && binary.LittleEndian.Uint32(relocByte[loc.Offset:]) == 0 {
			// Maybe a CGo call
			opcode = bytes[1]
			copy(bytes, x86amd64JMPNearCode)
		} else if bytes[1] == x86amd64JMPcode {
			opcode = bytes[1]
		} else {
			return fmt.Errorf("do not support x86 opcode: %x for symbol %s (offset %d)!\n", relocByte[loc.Offset-2:loc.Offset+loc.Size], loc.Sym.Name, offset)
		}
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(relocToEpilogueOffset))
		switch opcode {
		case x86amd64CMPLcode:
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceCMPLcode)
			segment.codeByte[epilogueOffset+15] = relocByte[loc.Offset+loc.Size] // The 8 bit number to compare against
			copy2Slice(segment.codeByte[epilogueOffset+3:], addr, PtrSize)
			epilogueOffset += len(x86amd64replaceCMPLcode)
		case x86amd64MOVcode:
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceMOVQcode)
			segment.codeByte[epilogueOffset+1] = register
			copy2Slice(segment.codeByte[epilogueOffset+2:], addr, PtrSize)
			epilogueOffset += len(x86amd64replaceMOVQcode)
		case x86amd64CALLcode:
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceCALLcode)
			copy2Slice(segment.codeByte[epilogueOffset+3:], addr, PtrSize)
			epilogueOffset += len(x86amd64replaceCALLcode)
		case x86amd64JMPcode:
			copy(segment.codeByte[epilogueOffset:], x86amd64JMPLcode)
			epilogueOffset += len(x86amd64JMPLcode)
			copy2Slice(segment.codeByte[epilogueOffset:], addr, PtrSize)
			epilogueOffset += PtrSize
		case x86amd64LEAcode:
			putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(addr))
		default:
			return fmt.Errorf("unexpected x86 opcode %x: %x for symbol %s (offset %d)!\n", opcode, relocByte[loc.Offset-2:loc.Offset+loc.Size], loc.Sym.Name, offset)
		}

		switch opcode {
		case x86amd64CMPLcode, x86amd64MOVcode, x86amd64CALLcode:
			returnOffset := (loc.Offset + loc.Size - loc.Add) - epilogueOffset
			if returnOffset > -0x80 && returnOffset < 0 {
				copy(segment.codeByte[epilogueOffset:], x86amd64JMPShortCode)
				segment.codeByte[epilogueOffset+1] = uint8(returnOffset)
				epilogueOffset += len(x86amd64JMPShortCode)
			} else {
				copy(segment.codeByte[epilogueOffset:], x86amd64JMPNearCode)
				byteorder.PutUint32(segment.codeByte[epilogueOffset+1:], uint32(returnOffset))
				epilogueOffset += len(x86amd64JMPNearCode)
			}
		}
	} else {
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
	}
	return err
}

func (linker *Linker) relocateCALLARM(addr uintptr, loc obj.Reloc, segment *segment) error {
	byteorder := linker.Arch.ByteOrder
	add := loc.Add
	if loc.Type == reloctype.R_CALLARM {
		add = int(signext24(int64(loc.Add&0xFFFFFF)) * 4)
	}
	epilogueOffset := loc.EpilogueOffset
	copy(segment.codeByte[epilogueOffset:epilogueOffset+loc.EpilogueSize], make([]byte, loc.EpilogueSize))
	offset := (int(addr) + add - (segment.codeBase + loc.Offset)) / 4
	if offset > 0x7FFFFF || offset < -0x800000 {
		if loc.EpilogueSize == 0 {
			return fmt.Errorf("relocation epilogue not available but got a >24-bit CALLARM reloc with offset %d: %s", offset, loc.Sym.Name)
		}
		off := uint32(epilogueOffset-loc.Offset) / 4
		if loc.Type == reloctype.R_CALLARM {
			add = int(signext24(int64(loc.Add&0xFFFFFF)+2) * 4)
			off = uint32(epilogueOffset-loc.Offset-8) / 4
		}
		putUint24(segment.codeByte[loc.Offset:], off)
		if loc.Type == reloctype.R_CALLARM64 {
			copy(segment.codeByte[epilogueOffset:], arm64CALLCode)
			epilogueOffset += len(arm64CALLCode)
		} else {
			copy(segment.codeByte[epilogueOffset:], armcode)
			epilogueOffset += len(armcode)
		}
		putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(int(addr)+add))
	} else {
		val := byteorder.Uint32(segment.codeByte[loc.Offset:])
		if loc.Type == reloctype.R_CALLARM {
			val |= uint32(offset) & 0x00FFFFFF
		} else {
			val |= uint32(offset) & 0x03FFFFFF
		}
		byteorder.PutUint32(segment.codeByte[loc.Offset:], val)
	}
	return nil
}

func (linker *Linker) relocate(codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	byteorder := linker.Arch.ByteOrder

	for _, symbol := range linker.symMap {
		for _, loc := range symbol.Reloc {
			addr := symbolMap[loc.Sym.Name]
			fmAddr, duplicated := symbolMap[FirstModulePrefix+loc.Sym.Name]
			if strings.HasPrefix(loc.Sym.Name, TypePrefix) && !duplicated {
				if variant, ok := symbolIsVariant(loc.Sym.Name); ok {
					fmAddr, duplicated = symbolMap[variant]
				}
			}
			if duplicated {
				isTypeWhichShouldNotBeDeduped := false
				for _, pkgPath := range linker.options.SkipTypeDeduplicationForPackages {
					if loc.Sym.Pkg == pkgPath {
						isTypeWhichShouldNotBeDeduped = true
					}
				}
				if !isTypeWhichShouldNotBeDeduped {
					// Always use the new module types initially - we will later check for type equality and
					// deduplicate them if they're structurally equal. If we used the firstmodule types here, there's a
					// risk they're not structurally equal, but it would be too late
					if !strings.HasPrefix(loc.Sym.Name, TypePrefix) {
						addr = fmAddr
					}
				}
			}
			sym := loc.Sym
			relocByte := segment.dataByte
			addrBase := segment.dataBase
			if symbol.Kind == symkind.STEXT {
				addrBase = segment.codeBase
				relocByte = segment.codeByte
			}
			if strings.HasPrefix(sym.Name, ItabPrefix) {
				isItabWhichShouldNotBeDeduped := false
				for _, pkgPath := range linker.options.SkipTypeDeduplicationForPackages {
					if strings.HasPrefix(strings.TrimLeft(strings.TrimPrefix(sym.Name, ItabPrefix), "*"), pkgPath) {
						isItabWhichShouldNotBeDeduped = true
					}
				}
				if addr == 0 || isItabWhichShouldNotBeDeduped {
					addr = uintptr(segment.dataBase + loc.Sym.Offset)
					symbolMap[loc.Sym.Name] = addr
					codeModule.module.itablinks = append(codeModule.module.itablinks, (*itab)(adduintptr(uintptr(segment.dataBase), loc.Sym.Offset)))
				}
			}

			if linker.options.RelocationDebugWriter != nil {
				isDup := "    "
				if duplicated {
					isDup = "DUP "
				}
				var weakness string
				if loc.Type&reloctype.R_WEAK > 0 {
					weakness = "WEAK|"
				}
				relocType := weakness + objabi.RelocType(loc.Type&^reloctype.R_WEAK).String()
				_, _ = fmt.Fprintf(linker.options.RelocationDebugWriter, "RELOCATING %s %10s %10s %18s Base: 0x%x Pos: 0x%08x, Addr: 0x%016x AddrFromBase: %12d %s   to    %s\n",
					isDup, objabi.SymKind(symbol.Kind), objabi.SymKind(sym.Kind), relocType, addrBase, uintptr(unsafe.Pointer(&relocByte[loc.Offset])),
					addr, int(addr)-addrBase, symbol.Name, sym.Name)
			}

			if addr != InvalidHandleValue {
				switch loc.Type {
				case reloctype.R_TLS_LE:
					if _, ok := symbolMap[TLSNAME]; !ok {
						symbolMap[TLSNAME] = tls.GetTLSOffset(linker.Arch, PtrSize)
					}
					byteorder.PutUint32(relocByte[loc.Offset:], uint32(symbolMap[TLSNAME]))
				case reloctype.R_CALL:
					err = linker.relocateCALL(addr, loc, segment, relocByte, addrBase)
				case reloctype.R_PCREL:
					err = linker.relocatePCREL(addr, loc, segment, relocByte, addrBase)
				case reloctype.R_CALLARM, reloctype.R_CALLARM64:
					err = linker.relocateCALLARM(addr, loc, segment)
				case reloctype.R_ADDRARM64, reloctype.R_ARM64_PCREL_LDST8, reloctype.R_ARM64_PCREL_LDST16, reloctype.R_ARM64_PCREL_LDST32, reloctype.R_ARM64_PCREL_LDST64:
					if symbol.Kind != symkind.STEXT {
						err = fmt.Errorf("impossible! Sym: %s is not in code segment! (kind %s)\n", sym.Name, objabi.SymKind(sym.Kind))
					}
					err = linker.relocateADRP(relocByte[loc.Offset:], loc, segment, addr)
				case reloctype.R_ADDR, reloctype.R_WEAKADDR:
					address := uintptr(int(addr) + loc.Add)
					putAddress(byteorder, relocByte[loc.Offset:], uint64(address))
				case reloctype.R_CALLIND:
					// nothing todo
				case reloctype.R_ADDROFF, reloctype.R_WEAKADDROFF:
					offset := int(addr) - addrBase + loc.Add
					if offset > 0x7FFFFFFF || offset < -0x80000000 {
						err = fmt.Errorf("symName: %s offset for %s: %d overflows!\n", sym.Name, objabi.RelocType(loc.Type), offset)
					}
					byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
				case reloctype.R_METHODOFF:
					if loc.Sym.Kind == symkind.STEXT {
						addrBase = segment.codeBase
					}
					offset := int(addr) - addrBase + loc.Add
					if offset > 0x7FFFFFFF || offset < -0x80000000 {
						err = fmt.Errorf("symName: %s offset for R_METHODOFF: %d overflows!\n", sym.Name, offset)
					}
					byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
				case reloctype.R_USETYPE:
					// nothing todo
				case reloctype.R_USEIFACE:
					// nothing todo
				case reloctype.R_USEIFACEMETHOD:
					// nothing todo
				case reloctype.R_ADDRCUOFF:
					// nothing todo
				case reloctype.R_KEEP:
					// nothing todo
				default:
					err = fmt.Errorf("unknown reloc type: %s sym: %s", objabi.RelocType(loc.Type).String(), sym.Name)
				}
			}
			if err != nil {
				return err
			}
		}
	}
	return err
}
