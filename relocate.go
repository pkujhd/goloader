package goloader

import (
	"cmd/objfile/objabi"
	"encoding/binary"
	"fmt"
	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/reloctype"
	"github.com/pkujhd/goloader/objabi/symkind"
	"github.com/pkujhd/goloader/objabi/tls"
	"strings"
	"unsafe"
)

const (
	maxExtraInstructionBytesADRP      = 16
	maxExtraInstructionBytesCALLARM64 = 16
	maxExtraInstructionBytesPCREL     = 48
	maxExtraInstructionBytesCALL      = 14
)

func (linker *Linker) relocateADRP(mCode []byte, loc obj.Reloc, segment *segment, symAddr uintptr) {
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
	// R_ADDRARM64 relocs include 2x 32 bit instructions, one ADRP, and one ADD - both contain the destination register in the lowest 5 bits
	if signedOffset > 1<<32 || signedOffset < -1<<32 {
		// Too far to fit inside an ADRP+ADD, do a jump to some extra code we add at the end big enough to fit any 64 bit address
		symAddr += uintptr(loc.Add)
		addr := byteorder.Uint32(mCode)
		bcode := byteorder.Uint32(arm64Bcode) // Unconditional branch
		bcode |= ((uint32(epilogueOffset) - uint32(loc.Offset)) >> 2) & 0x01FFFFFF
		if epilogueOffset-loc.Offset < 0 {
			bcode |= 0x02000000 // 26th bit is sign bit
		}
		byteorder.PutUint32(mCode, bcode) // The second ADD instruction in the ADRP reloc will be bypassed as we return from the jump after it

		ldrCode := uint32(0x58000040) // LDR PC+8
		ldrCode |= addr & 0x1F        // Set the register

		byteorder.PutUint32(segment.codeByte[epilogueOffset:], ldrCode)
		epilogueOffset += Uint32Size

		bcode = byteorder.Uint32(arm64Bcode)
		bcode |= ((uint32(loc.Offset) - uint32(epilogueOffset) + 8) >> 2) & 0x01FFFFFF
		if loc.Offset-epilogueOffset+8 < 0 {
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
		add := byteorder.Uint32(mCode[4:8])
		add |= uint32(uint64(signedOffset)&0xFFF) << 10
		byteorder.PutUint32(mCode, adrp)
		byteorder.PutUint32(mCode[4:], add)
	}
}

func (linker *Linker) relocateCALL(addr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) {
	byteorder := linker.Arch.ByteOrder
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	epilogueOffset := loc.EpilogueOffset
	copy(segment.codeByte[epilogueOffset:epilogueOffset+loc.EpilogueSize], make([]byte, loc.EpilogueSize))

	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		offset = (segment.codeBase + epilogueOffset) - (addrBase + loc.Offset + loc.Size)
		copy(segment.codeByte[epilogueOffset:], x86amd64JMPLcode)
		epilogueOffset += len(x86amd64JMPLcode)
		putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(addr)+uint64(loc.Add))
	}
	byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
}

func (linker *Linker) relocatePCREL(addr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) (err error) {
	byteorder := linker.Arch.ByteOrder
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	epilogueOffset := loc.EpilogueOffset
	if oldMcode, ok := linker.appliedPCRelRelocs[&relocByte[loc.Offset-2]]; !ok {
		linker.appliedPCRelRelocs[&relocByte[loc.Offset-2]] = make([]byte, loc.Size+2)
		copy(linker.appliedPCRelRelocs[&relocByte[loc.Offset-2]], relocByte[loc.Offset-2:])
	} else {
		copy(relocByte[loc.Offset-2:], oldMcode)
	}
	copy(segment.codeByte[epilogueOffset:epilogueOffset+loc.EpilogueSize], make([]byte, loc.EpilogueSize))
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		offset = (segment.codeBase + epilogueOffset) - (addrBase + loc.Offset + loc.Size)
		bytes := relocByte[loc.Offset-2:]
		opcode := relocByte[loc.Offset-2]
		regsiter := ZeroByte
		if opcode == x86amd64LEAcode {
			bytes[0] = x86amd64MOVcode
		} else if opcode == x86amd64MOVcode && loc.Size >= Uint32Size {
			regsiter = ((relocByte[loc.Offset-1] >> 3) & 0x7) | 0xb8
			copy(bytes, x86amd64JMPLcode)
		} else if opcode == x86amd64CMPLcode && loc.Size >= Uint32Size {
			copy(bytes, x86amd64JMPLcode)
		} else if (bytes[1] == x86amd64CALLcode) && binary.LittleEndian.Uint32(relocByte[loc.Offset:]) == 0 {
			// Maybe a CGo call
			copy(bytes, x86amd64JMPNearCode)
			opcode = bytes[1]
			byteorder.PutUint32(bytes[1:], uint32(offset))
		} else if bytes[1] == x86amd64JMPcode && offset < 1<<32 {
			byteorder.PutUint32(bytes[1:], uint32(offset))
		} else {
			return fmt.Errorf("do not support x86 opcode: %x for symbol %s (offset %d)!\n", relocByte[loc.Offset-2:loc.Offset], loc.Sym.Name, offset)
		}
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
		if opcode == x86amd64CMPLcode || opcode == x86amd64MOVcode {
			putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(segment.codeBase+epilogueOffset+PtrSize))
			if opcode == x86amd64CMPLcode {
				copy(segment.codeByte[epilogueOffset:], x86amd64replaceCMPLcode)
				segment.codeByte[epilogueOffset+0x0F] = relocByte[loc.Offset+loc.Size]
				epilogueOffset += len(x86amd64replaceCMPLcode)
				putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(addr))
			} else {
				copy(segment.codeByte[epilogueOffset:], x86amd64replaceMOVQcode)
				segment.codeByte[epilogueOffset+1] = regsiter
				copy2Slice(segment.codeByte[epilogueOffset+2:], addr, PtrSize)
				epilogueOffset += len(x86amd64replaceMOVQcode)
			}
			putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(addrBase+loc.Offset+loc.Size-loc.Add))
		} else if opcode == x86amd64CALLcode {
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceCALLcode)
			byteorder.PutUint64(segment.codeByte[epilogueOffset+4:], uint64(addr))
			epilogueOffset += len(x86amd64replaceCALLcode)
			copy(segment.codeByte[epilogueOffset:], x86amd64JMPNearCode)
			byteorder.PutUint32(segment.codeByte[epilogueOffset+1:], uint32(offset))
			epilogueOffset += len(x86amd64JMPNearCode)
		} else {
			putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(addr))
		}
	} else {
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
	}
	return err
}

func (linker *Linker) relocateCALLARM(addr uintptr, loc obj.Reloc, segment *segment) {
	byteorder := linker.Arch.ByteOrder
	add := loc.Add
	if loc.Type == reloctype.R_CALLARM {
		add = int(signext24(int64(loc.Add&0xFFFFFF)) * 4)
	}
	epilogueOffset := loc.EpilogueOffset
	copy(segment.codeByte[epilogueOffset:epilogueOffset+loc.EpilogueSize], make([]byte, loc.EpilogueSize))
	offset := (int(addr) + add - (segment.codeBase + loc.Offset)) / 4
	if offset > 0x7FFFFF || offset < -0x800000 {
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
}

func (linker *Linker) relocate(codeModule *CodeModule, symbolMap map[string]uintptr) (err error) {
	segment := &codeModule.segment
	byteorder := linker.Arch.ByteOrder

	for _, symbol := range linker.symMap {
		for _, loc := range symbol.Reloc {
			addr := symbolMap[loc.Sym.Name]
			fmAddr, duplicated := symbolMap[FirstModulePrefix+loc.Sym.Name]
			if duplicated {
				addr = fmAddr
			}
			sym := loc.Sym
			relocByte := segment.dataByte
			addrBase := segment.dataBase
			if symbol.Kind == symkind.STEXT {
				addrBase = segment.codeBase
				relocByte = segment.codeByte
			}
			if addr == 0 && strings.HasPrefix(sym.Name, ItabPrefix) {
				addr = uintptr(segment.dataBase + loc.Sym.Offset)
				symbolMap[loc.Sym.Name] = addr
				codeModule.module.itablinks = append(codeModule.module.itablinks, (*itab)(adduintptr(uintptr(segment.dataBase), loc.Sym.Offset)))
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
					linker.relocateCALL(addr, loc, segment, relocByte, addrBase)
				case reloctype.R_PCREL:
					err = linker.relocatePCREL(addr, loc, segment, relocByte, addrBase)
				case reloctype.R_CALLARM, reloctype.R_CALLARM64:
					linker.relocateCALLARM(addr, loc, segment)
				case reloctype.R_ADDRARM64:
					if symbol.Kind != symkind.STEXT {
						err = fmt.Errorf("impossible! Sym: %s is not in code segment! (kind %s)\n", sym.Name, objabi.SymKind(sym.Kind))
					}
					linker.relocateADRP(relocByte[loc.Offset:], loc, segment, addr)
				case reloctype.R_ADDR, reloctype.R_WEAKADDR:
					address := uintptr(int(addr) + loc.Add)
					putAddress(byteorder, relocByte[loc.Offset:], uint64(address))
				case reloctype.R_CALLIND:
					//nothing todo
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
					//nothing todo
				case reloctype.R_USEIFACE:
					//nothing todo
				case reloctype.R_USEIFACEMETHOD:
					//nothing todo
				case reloctype.R_ADDRCUOFF:
					//nothing todo
				case reloctype.R_KEEP:
					//nothing todo
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
