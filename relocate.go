package goloader

import (
	"cmd/objfile/sys"
	"fmt"
	"runtime"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/reloctype"
	"github.com/pkujhd/goloader/objabi/symkind"
)

const (
	maxExtraCodeSize_ADDRARM64        = 24
	maxExtraCodeSize_CALLARM64        = 16
	maxExtraCodeSize_ARM64_PCREL_LDST = 24
	maxExtraCodeSize_PCRELxMOV        = 18
	maxExtraCodeSize_PCRELxCMPL       = 21
	maxExtraCodeSize_PCRELxCALL       = 11
	maxExtraCodeSize_PCRELxJMP        = 14
	maxExtraCodeSize_CALL             = 11
)

func expandFunc(linker *Linker, objsym *obj.ObjSymbol, symbol *obj.Sym) {
	// on linux/amd64, mmap force return < 32bit address, don't need add extra instructions
	if linker.Arch.Name == sys.ArchAMD64.Name && runtime.GOOS == "linux" {
		return
	}
	// Pessimistically pad the function text with extra bytes for any relocations which might add extra
	// instructions at the end in the case of a 32 bit overflow. These epilogue PCs need to be added to
	// the PCData, PCLine, PCFile, PCSP etc.
	for i, reloc := range objsym.Reloc {
		epilogue := &(objsym.Reloc[i].Epilogue)
		epilogue.Offset = len(linker.Code) - symbol.Offset
		switch reloc.Type {
		case reloctype.R_ADDRARM64:
			epilogue.Size = maxExtraCodeSize_ADDRARM64
		case reloctype.R_CALLARM64:
			epilogue.Size = maxExtraCodeSize_CALLARM64
		case reloctype.R_ARM64_PCREL_LDST8, reloctype.R_ARM64_PCREL_LDST16, reloctype.R_ARM64_PCREL_LDST32, reloctype.R_ARM64_PCREL_LDST64:
			epilogue.Size = maxExtraCodeSize_ARM64_PCREL_LDST
		case reloctype.R_CALL:
			epilogue.Size = maxExtraCodeSize_CALL
		case reloctype.R_PCREL:
			opCodes := objsym.Data[reloc.Offset-2 : reloc.Offset+reloc.Size]
			switch opCodes[0] {
			case x86amd64MOVcode:
				epilogue.Size = maxExtraCodeSize_PCRELxMOV
			case x86amd64CMPLcode:
				epilogue.Size = maxExtraCodeSize_PCRELxCMPL
			default:
				switch opCodes[1] {
				case x86amd64CALLcode:
					epilogue.Size = maxExtraCodeSize_PCRELxCALL
				case x86amd64JMPcode:
					epilogue.Size = maxExtraCodeSize_PCRELxJMP
				}
			}
		}
		if epilogue.Size > 0 {
			linker.Code = append(linker.Code, createArchNops(linker.Arch, epilogue.Size)...)
		}
	}
}

func (linker *Linker) relocateADRP(mCode []byte, loc obj.Reloc, segment *segment, symAddr uintptr) (err error) {
	byteorder := linker.Arch.ByteOrder
	offset := int64(symAddr) + int64(loc.Add) - ((int64(segment.codeBase) + int64(loc.Offset)) &^ 0xFFF)
	//overflow
	if offset >= 1<<32 || offset < -1<<32 {
		epilogueOffset := loc.Epilogue.Offset
		symAddr = symAddr + uintptr(loc.Add)
		if symAddr < 0xFFFFFFFF && loc.Type == reloctype.R_ADDRARM64 {
			addr := byteorder.Uint32(mCode)
			//low:	MOV reg imm
			low := uint32(0xD2800000)
			//high: MOVK reg imm LSL#16
			high := uint32(0xF2A00000)
			low = ((addr & 0x1F) | low) | ((uint32(symAddr) & 0xFFFF) << 5)
			high = ((addr & 0x1F) | high) | (uint32(symAddr) >> 16 << 5)
			byteorder.PutUint64(mCode, uint64(low)|(uint64(high)<<32))
		} else {
			addr := byteorder.Uint32(mCode)
			if loc.Type != reloctype.R_ADDRARM64 {
				addr = uint32(byteorder.Uint64(mCode) >> 32)
			}
			blcode := byteorder.Uint32(arm64Bcode)
			blcode |= ((uint32(epilogueOffset) - uint32(loc.Offset)) >> 2) & 0x01FFFFFF
			if epilogueOffset-loc.Offset < 0 {
				blcode |= 0x02000000
			}
			byteorder.PutUint32(mCode, blcode)
			//low: MOV reg imm
			llow := uint32(0xD2800000)
			//lhigh: MOVK reg imm LSL#16
			lhigh := uint32(0xF2A00000)
			//llow: MOVK reg imm LSL#32
			hlow := uint32(0xF2C00000)
			//lhigh: MOVK reg imm LSL#48
			hhigh := uint32(0xF2E00000)
			llow = ((addr & 0x1F) | llow) | ((uint32(symAddr) & 0xFFFF) << 5)
			lhigh = ((addr & 0x1F) | lhigh) | (uint32(symAddr) >> 16 << 5)
			putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(llow)|(uint64(lhigh)<<32))
			hlow = ((addr & 0x1F) | hlow) | uint32(((uint64(symAddr)>>32)&0xFFFF)<<5)
			hhigh = ((addr & 0x1F) | hhigh) | uint32((uint64(symAddr)>>48)<<5)
			putAddressAddOffset(byteorder, segment.codeByte, &epilogueOffset, uint64(hlow)|(uint64(hhigh)<<32))
			if loc.Type != reloctype.R_ADDRARM64 {
				//LDR or STR
				ldrOrStr := (byteorder.Uint32(mCode[4:]) & 0xFFFFFC00) | addr&0x1F | ((addr & 0x1F) << 5)
				byteorder.PutUint32(segment.codeByte[epilogueOffset:], ldrOrStr)
				epilogueOffset += Uint32Size
			}
			blcode = byteorder.Uint32(arm64Bcode)
			blcode |= ((uint32(loc.Offset) - uint32(epilogueOffset) + PtrSize) >> 2) & 0x01FFFFFF
			if loc.Offset-epilogueOffset+PtrSize < 0 {
				blcode |= 0x02000000
			}
			byteorder.PutUint32(segment.codeByte[epilogueOffset:], blcode)
		}
	} else {
		// 2bit + 19bit + low(12bit) = 33bit
		low := (uint32((offset>>12)&3) << 29) | (uint32((offset>>12>>2)&0x7FFFF) << 5)
		high := uint32(0)
		switch loc.Type {
		case reloctype.R_ADDRARM64, reloctype.R_ARM64_PCREL_LDST8:
			high = uint32(offset&0xFFF) << 10
		case reloctype.R_ARM64_PCREL_LDST16:
			if offset&0x1 != 0 {
				err = fmt.Errorf("offset for 16-bit load/store has unaligned value %d", offset&0xFFF)
			}
			high = (uint32(offset&0xFFF) >> 1) << 10
		case reloctype.R_ARM64_PCREL_LDST32:
			if offset&0x3 != 0 {
				err = fmt.Errorf("offset for 32-bit load/store has unaligned value %d", offset&0xFFF)
			}
			high = (uint32(offset&0xFFF) >> 2) << 10
		case reloctype.R_ARM64_PCREL_LDST64:
			if offset&0x7 != 0 {
				err = fmt.Errorf("offset for 64-bit load/store has unaligned value %d", offset&0xFFF)
			}
			high = (uint32(offset&0xFFF) >> 3) << 10
		}
		value := byteorder.Uint64(mCode)
		value = (uint64(uint32(value>>32)|high) << 32) | uint64(uint32(value&0xFFFFFFFF)|low)
		byteorder.PutUint64(mCode, value)
	}
	return err
}

func (linker *Linker) relocateCALL(addr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) {
	byteorder := linker.Arch.ByteOrder
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	if isOverflowInt32(offset) {
		segment.dataOff = alignof(segment.dataOff, PtrSize)
		epilogueOffset := loc.Epilogue.Offset
		offset = (segment.codeBase + epilogueOffset) - (addrBase + loc.Offset + loc.Size)
		relocByte[loc.Offset-1] = x86amd64JMPcode
		copy(segment.codeByte[epilogueOffset:], x86amd64replaceCALLcode)
		off := (segment.dataBase + segment.dataOff - segment.codeBase - epilogueOffset - 6)
		byteorder.PutUint32(segment.codeByte[epilogueOffset+2:], uint32(off))
		putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(addr)+uint64(loc.Add))
		epilogueOffset += len(x86amd64replaceCALLcode)
		off = addrBase + loc.Offset + loc.Size - segment.codeBase - epilogueOffset
		byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(off))
	}
	byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
}

func (linker *Linker) relocatePCREL(addr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) (err error) {
	byteorder := linker.Arch.ByteOrder
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	if isOverflowInt32(offset) {
		epilogueOffset := loc.Epilogue.Offset
		offset = (segment.codeBase + epilogueOffset) - (addrBase + loc.Offset + loc.Size)
		bytes := relocByte[loc.Offset-2:]
		opcode := relocByte[loc.Offset-2]
		register := ZeroByte
		if opcode == x86amd64LEAcode {
			bytes[0] = x86amd64MOVcode
			//not append epilogue for LEA, put the address into data segment.
			offset = (segment.dataBase + segment.dataOff) - (addrBase + loc.Offset + loc.Size)
		} else if opcode == x86amd64MOVcode && loc.Size >= Uint32Size {
			register = (relocByte[loc.Offset-1] >> 3) & 0x7
			copy(bytes, append(x86amd64NOPcode, x86amd64JMPNcode...))
		} else if opcode == x86amd64CMPLcode && loc.Size >= Uint32Size {
			copy(bytes, append(x86amd64NOPcode, x86amd64JMPNcode...))
		} else if (bytes[1] == x86amd64CALLcode) && byteorder.Uint32(relocByte[loc.Offset:]) == 0 {
			opcode = bytes[1]
		} else if bytes[1] == x86amd64JMPcode {
			opcode = bytes[1]
		} else {
			return fmt.Errorf("do not support x86 opcode: %x for symbol %s (offset %d)!\n", relocByte[loc.Offset-2:loc.Offset], loc.Sym.Name, offset)
		}
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
		switch opcode {
		case x86amd64CMPLcode:
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceCMPLcode)
			segment.codeByte[epilogueOffset+0x11] = relocByte[loc.Offset+loc.Size]
			putAddress(byteorder, segment.codeByte[epilogueOffset+3:], uint64(addr)+uint64(loc.Add))
			epilogueOffset += len(x86amd64replaceCMPLcode)
		case x86amd64MOVcode:
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceMOVQcode)
			segment.codeByte[epilogueOffset+1] |= register
			segment.codeByte[epilogueOffset+0xC] |= register | (register << 3)
			putAddress(byteorder, segment.codeByte[epilogueOffset+2:], uint64(addr)+uint64(loc.Add))
			epilogueOffset += len(x86amd64replaceMOVQcode)
		case x86amd64CALLcode:
			segment.dataOff = alignof(segment.dataOff, PtrSize)
			bytes[1] = x86amd64JMPcode
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceCALLcode)
			off := (segment.dataBase + segment.dataOff - segment.codeBase - epilogueOffset - 6)
			byteorder.PutUint32(segment.codeByte[epilogueOffset+2:], uint32(off))
			epilogueOffset += len(x86amd64replaceCALLcode)
			putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(addr)+uint64(loc.Add))
			off = addrBase + loc.Offset + loc.Size - segment.codeBase - epilogueOffset
			byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(off))
		case x86amd64JMPcode:
			copy(segment.codeByte[epilogueOffset:], x86amd64JMPLcode)
			epilogueOffset += len(x86amd64JMPLcode)
			copy2Slice(segment.codeByte[epilogueOffset:], addr, PtrSize)
			epilogueOffset += PtrSize
		case x86amd64LEAcode:
			putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(addr)+uint64(loc.Add))
		default:
			return fmt.Errorf("unexpected x86 opcode %x: %x for symbol %s (offset %d)!\n", opcode, relocByte[loc.Offset-2:loc.Offset], loc.Sym.Name, offset)
		}

		switch opcode {
		case x86amd64CMPLcode, x86amd64MOVcode:
			copy(segment.codeByte[epilogueOffset:], x86amd64JMPNcode)
			epilogueOffset += len(x86amd64JMPNcode)
			returnOffset := (loc.Offset + loc.Size - loc.Add) - epilogueOffset
			byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(returnOffset))
		}
	} else {
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
	}
	return nil
}

func (linker *Linker) relocteCALLARM(addr uintptr, loc obj.Reloc, segment *segment) {
	byteorder := linker.Arch.ByteOrder
	add := loc.Add
	if loc.Type == reloctype.R_CALLARM {
		add = int(signext24(int64(loc.Add&0xFFFFFF)) * 4)
	}
	offset := (int(addr) + add - (segment.codeBase + loc.Offset)) / 4
	if isOverflowInt24(offset) {
		epilogueOffset := loc.Epilogue.Offset
		off := uint32(epilogueOffset-loc.Offset) / 4
		if loc.Type == reloctype.R_CALLARM {
			add = int(signext24(int64(loc.Add&0xFFFFFF)+2) * 4)
			off = uint32(epilogueOffset-loc.Offset-8) / 4
		}
		putUint24(segment.codeByte[loc.Offset:], off)
		if loc.Type == reloctype.R_CALLARM64 {
			copy(segment.codeByte[epilogueOffset:], arm64ReplaceCALLCode)
			epilogueOffset += len(arm64ReplaceCALLCode)
		} else {
			copy(segment.codeByte[epilogueOffset:], armReplaceCallCode)
			epilogueOffset += len(armReplaceCallCode)
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

func (linker *Linker) relocate(codeModule *CodeModule, symbolMap, symPtr map[string]uintptr) (err error) {
	segment := &codeModule.segment
	byteorder := linker.Arch.ByteOrder
	for _, symbol := range linker.SymMap {
		for _, loc := range symbol.Reloc {
			addr := symbolMap[loc.Sym.Name]
			sym := loc.Sym
			relocByte := segment.dataByte
			addrBase := segment.dataBase
			if symbol.Kind == symkind.STEXT {
				addrBase = segment.codeBase
				relocByte = segment.codeByte
			}

			if addr != InvalidHandleValue {
				switch loc.Type {
				case reloctype.R_TLS_LE:
					byteorder.PutUint32(relocByte[loc.Offset:], uint32(symbolMap[TLSNAME]))
				case reloctype.R_CALL, reloctype.R_CALL | reloctype.R_WEAK:
					linker.relocateCALL(addr, loc, segment, relocByte, addrBase)
				case reloctype.R_PCREL:
					err = linker.relocatePCREL(addr, loc, segment, relocByte, addrBase)
				case reloctype.R_CALLARM, reloctype.R_CALLARM64, reloctype.R_CALLARM64 | reloctype.R_WEAK:
					linker.relocteCALLARM(addr, loc, segment)
				case reloctype.R_ADDRARM64, reloctype.R_ARM64_PCREL_LDST8, reloctype.R_ARM64_PCREL_LDST16, reloctype.R_ARM64_PCREL_LDST32, reloctype.R_ARM64_PCREL_LDST64:
					if symbol.Kind != symkind.STEXT {
						err = fmt.Errorf("impossible!Sym:%s locate not in code segment!\n", sym.Name)
					}
					err = linker.relocateADRP(relocByte[loc.Offset:], loc, segment, addr)
				case reloctype.R_ADDR, reloctype.R_WEAKADDR:
					address := uintptr(int(addr) + loc.Add)
					putAddress(byteorder, relocByte[loc.Offset:], uint64(address))
				case reloctype.R_CALLIND:
					//nothing todo
				case reloctype.R_ADDROFF, reloctype.R_WEAKADDROFF:
					offset := int(addr) - addrBase + loc.Add
					if isOverflowInt32(offset) {
						err = fmt.Errorf("symName:%s relocateType:%s, offset:%d is overflow!\n", sym.Name, reloctype.RelocTypeString(loc.Type), offset)
					}
					byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
				case reloctype.R_METHODOFF:
					if loc.Sym.Kind == symkind.STEXT {
						addrBase = segment.codeBase
					}
					offset := int(addr) - addrBase + loc.Add
					if isOverflowInt32(offset) {
						err = fmt.Errorf("symName:%s relocateType:%s, offset:%d is overflow!\n", sym.Name, reloctype.RelocTypeString(loc.Type), offset)
					}
					byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
				case reloctype.R_GOTPCREL, reloctype.R_ARM64_GOTPCREL:
					offset := uint32(segment.dataBase + segment.dataOff - addrBase - loc.Offset - loc.Size)
					byteorder.PutUint32(relocByte[loc.Offset:], offset)
					putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(addr))
				case reloctype.R_USETYPE,
					reloctype.R_USEIFACE,
					reloctype.R_USEIFACEMETHOD,
					reloctype.R_ADDRCUOFF,
					reloctype.R_INITORDER:
					//nothing todo
				default:
					err = fmt.Errorf("unknown reloc type:%s sym:%s", reloctype.RelocTypeString(loc.Type), sym.Name)
				}
			}
			if err != nil {
				return err
			}
		}
	}
	return err
}
