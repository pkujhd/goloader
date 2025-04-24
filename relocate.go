package goloader

import (
	"encoding/binary"
	"fmt"
	"github.com/pkujhd/goloader/constants"
	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/reloctype"
	"github.com/pkujhd/goloader/objabi/symkind"
	"github.com/pkujhd/goloader/objabi/tls"
	"strings"
)

const (
	maxExtraCodeSize_ADDRARM64        = 24
	maxExtraCodeSize_CALLARM64        = 16
	maxExtraCodeSize_ARM64_PCREL_LDST = 24
	maxExtraCodeSize_PCRELxMOV        = 18
	maxExtraCodeSize_PCRELxCMPL       = 14
	maxExtraCodeSize_PCRELxCALL       = 11
	maxExtraCodeSize_PCRELxJMP        = 6
	maxExtraCodeSize_CALL             = 11
)

func expandFunc(linker *Linker, objsym *obj.ObjSymbol, symbol *obj.Sym) {
	// Pessimistically pad the function text with extra bytes for any relocations which might add extra
	// instructions at the end in the case of a 32 bit overflow. These epilogue PCs need to be added to
	// the PCData, PCLine, PCFile, PCSP etc.
	for i, reloc := range objsym.Reloc {
		// on linux/amd64, mmap force return < 32bit address,
		// doesn't need to add extra instructions except relocate symbol is a string.
		// because string is dynamic allocate in a far address
		if !isMmapInLowAddress(linker.Arch.Name) || strings.HasPrefix(reloc.SymName, constants.TypeStringPrefix) {
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
				linker.ExtraData += PtrSize
			case reloctype.R_PCREL:
				switch obj.GetOpName(reloc.Op) {
				case "LEA":
					linker.ExtraData += PtrSize
				case "MOV", "MOVUPS", "MOVZ", "MOVZX", "MOVQ", "MOVSD_XMM":
					epilogue.Size = maxExtraCodeSize_PCRELxMOV
				case "CMP", "CMPL":
					epilogue.Size = maxExtraCodeSize_PCRELxCMPL
				case "CALL":
					epilogue.Size = maxExtraCodeSize_PCRELxCALL
					linker.ExtraData += PtrSize
				case "JMP":
					epilogue.Size = maxExtraCodeSize_PCRELxJMP
					linker.ExtraData += PtrSize
				default:
				}
			case reloctype.R_GOTPCREL, reloctype.R_ARM64_GOTPCREL:
				linker.ExtraData += PtrSize
			}

			if epilogue.Size > 0 {
				linker.Code = append(linker.Code, createArchNops(linker.Arch, epilogue.Size)...)
			}
		}
	}
}

func (linker *Linker) relocateADRP(mCode []byte, loc obj.Reloc, segment *segment, symAddr uintptr) (err error) {
	byteorder := linker.Arch.ByteOrder
	offset := int64(symAddr) - ((int64(segment.codeBase) + int64(loc.Offset)) &^ 0xFFF)
	//overflow
	if offset >= 1<<32 || offset < -1<<32 {
		epilogueOffset := loc.Epilogue.Offset
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

func (linker *Linker) relocateCALL(symAddr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) error {
	byteorder := linker.Arch.ByteOrder
	offset := int(symAddr) - (addrBase + loc.Offset + loc.Size)
	if isOverflowInt32(offset) {
		epilogueOffset := loc.Epilogue.Offset
		switch obj.GetOpName(loc.Op) {
		case "CALL":
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceCALLcode)
			off := segment.dataBase + segment.dataOff - (segment.codeBase + epilogueOffset + len(x86amd64CALLcode))
			byteorder.PutUint32(segment.codeByte[epilogueOffset+2:], uint32(off))
			putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(symAddr))
			epilogueOffset += len(x86amd64replaceCALLcode)
			off = addrBase + loc.GetEnd() - segment.codeBase - epilogueOffset
			byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(off))
			fillCode(relocByte, loc, x86amd64JMPNcode, byteorder, loc.Epilogue.Offset-loc.GetEnd())
		case "JMP":
			copy(segment.codeByte[epilogueOffset:], x86amd64JMPLcode)
			epilogueOffset += len(x86amd64JMPLcode)
			off := segment.dataBase + segment.dataOff - (segment.codeBase + epilogueOffset)
			byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(off))
			putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(symAddr))
			byteorder.PutUint32(relocByte[loc.Offset:], uint32(loc.Epilogue.Offset-loc.GetEnd()))
		default:
			return fmt.Errorf("do not support x86 opcode:%s(Inst:%x) for symbol %s on CALL!\n", obj.GetOpName(loc.Op), loc.Text, loc.SymName)
		}
	} else {
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
	}

	return nil
}

func fillCode(relocByte []byte, reloc obj.Reloc, codes []byte, byteorder binary.ByteOrder, offset int) {
	if len(reloc.Text) < len(codes) {
		panic("not enough space for replace codes")
	}

	startPc := reloc.GetStart()
	for n := 0; n < len(reloc.Text)-len(codes); n++ {
		relocByte[startPc] = x86amd64NOPcode
		startPc++
	}
	copy(relocByte[startPc:], codes)
	byteorder.PutUint32(relocByte[reloc.GetEnd()-Uint32Size:], uint32(offset))
}

func (linker *Linker) relocatePCREL(symAddr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) (err error) {
	byteorder := linker.Arch.ByteOrder
	offset := int(symAddr) - (addrBase + loc.Offset + loc.Size)
	if isOverflowInt32(offset) {
		epilogueOffset := loc.Epilogue.Offset
		switch obj.GetOpName(loc.Op) {
		case "LEA":
			relocByte[loc.Offset-2] = x86amd64MOVcode
			//not append epilogue for LEA, put the address into data segment.
			offset = (segment.dataBase + segment.dataOff) - (addrBase + loc.GetEnd())
			byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
			putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(symAddr))
		case "MOV", "MOVUPS", "MOVZ", "MOVZX", "MOVQ", "MOVSD_XMM":
			register := (relocByte[loc.Offset-1] >> 3) & 0x7
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceMOVcode)
			if obj.IsExtraRegister(loc.Args[0]) {
				segment.codeByte[epilogueOffset] = 0x49
				segment.codeByte[epilogueOffset+10] = 0x4d
			}
			segment.codeByte[epilogueOffset+1] |= register
			segment.codeByte[epilogueOffset+12] |= register | (register << 3)
			putAddress(byteorder, segment.codeByte[epilogueOffset+2:], uint64(symAddr))
			epilogueOffset += len(x86amd64replaceMOVcode)
			byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(loc.GetEnd()-epilogueOffset))
			fillCode(relocByte, loc, x86amd64JMPNcode, byteorder, loc.Epilogue.Offset-loc.GetEnd())
		case "CMP", "CMPL":
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceCMPcode)
			byteorder.PutUint32(segment.codeByte[epilogueOffset+4:], uint32(symAddr))
			segment.codeByte[epilogueOffset+8] = relocByte[loc.Offset+loc.Size]
			epilogueOffset += len(x86amd64replaceCMPcode)
			byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(loc.GetEnd()-epilogueOffset))
			fillCode(relocByte, loc, x86amd64JMPNcode, byteorder, loc.Epilogue.Offset-loc.GetEnd())
		case "JMP":
			byteorder.PutUint32(relocByte[loc.Offset:], uint32(epilogueOffset-loc.GetEnd()))
			copy(segment.codeByte[epilogueOffset:], x86amd64JMPLcode)
			epilogueOffset += len(x86amd64JMPLcode)
			off := segment.dataBase + segment.dataOff - (segment.codeBase + epilogueOffset)
			byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(off))
			putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(symAddr))
		case "CALL":
			copy(segment.codeByte[epilogueOffset:], x86amd64replaceCALLcode)
			off := segment.dataBase + segment.dataOff - (segment.codeBase + epilogueOffset + len(x86amd64CALLcode))
			byteorder.PutUint32(segment.codeByte[epilogueOffset+2:], uint32(off))
			epilogueOffset += len(x86amd64replaceCALLcode)
			putAddressAddOffset(byteorder, segment.dataByte, &segment.dataOff, uint64(symAddr))
			byteorder.PutUint32(segment.codeByte[epilogueOffset-4:], uint32(loc.GetEnd()-epilogueOffset))
			fillCode(relocByte, loc, x86amd64JMPNcode, byteorder, loc.Epilogue.Offset-loc.GetEnd())
		default:
			return fmt.Errorf("do not support x86 opcode:%s(Inst:%x) for symbol %s on PCREL!\n", obj.GetOpName(loc.Op), loc.Text, loc.SymName)
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
	byteOrder := linker.Arch.ByteOrder
	tlsOffset := uint32(tls.GetTLSOffset(linker.Arch, linker.Arch.PtrSize))
	for _, symbol := range linker.SymMap {
		ifaceTypeMap := getUseIfaceTypeMap(symbol)
		for _, loc := range symbol.Reloc {
			symAddr := symbolMap[loc.SymName]
			if isItabName(loc.SymName) && isUseIfaceMethod(ifaceTypeMap, &loc) {
				if sym, ok := linker.SymMap[loc.SymName]; ok {
					symAddr = uintptr(sym.Offset + segment.dataBase)
				}
			}
			relocByte := segment.dataByte
			addrBase := segment.dataBase
			if symbol.Kind == symkind.STEXT {
				addrBase = segment.codeBase
				relocByte = segment.codeByte
			}

			if symAddr != InvalidHandleValue {
				symAddr = uintptr(int(symAddr) + loc.Add)
				switch loc.Type {
				case reloctype.R_TLS_LE, reloctype.R_TLS_IE:
					byteOrder.PutUint32(relocByte[loc.Offset:], tlsOffset)
				case reloctype.R_CALL, reloctype.R_CALL | reloctype.R_WEAK:
					err = linker.relocateCALL(symAddr, loc, segment, relocByte, addrBase)
				case reloctype.R_PCREL:
					err = linker.relocatePCREL(symAddr, loc, segment, relocByte, addrBase)
				case reloctype.R_CALLARM, reloctype.R_CALLARM64, reloctype.R_CALLARM64 | reloctype.R_WEAK:
					linker.relocteCALLARM(symbolMap[loc.SymName], loc, segment)
				case reloctype.R_ADDRARM64, reloctype.R_ARM64_PCREL_LDST8, reloctype.R_ARM64_PCREL_LDST16, reloctype.R_ARM64_PCREL_LDST32, reloctype.R_ARM64_PCREL_LDST64:
					if symbol.Kind != symkind.STEXT {
						err = fmt.Errorf("impossible!Sym:%s locate not in code segment!\n", loc.SymName)
					}
					err = linker.relocateADRP(relocByte[loc.Offset:], loc, segment, symAddr)
				case reloctype.R_ADDR, reloctype.R_WEAKADDR:
					putAddress(byteOrder, relocByte[loc.Offset:], uint64(symAddr))
				case reloctype.R_CALLIND:
					//nothing todo
				case reloctype.R_ADDROFF, reloctype.R_WEAKADDROFF:
					offset := int(symAddr) - addrBase
					if isOverflowInt32(offset) {
						if sym, ok := linker.SymMap[loc.SymName]; ok {
							offset = sym.Offset + loc.Add
						} else {
							err = fmt.Errorf("symName:%s relocateType:%s, offset:%d is overflow!\n", loc.SymName, reloctype.RelocTypeString(loc.Type), offset)
						}
					}
					byteOrder.PutUint32(relocByte[loc.Offset:], uint32(offset))
				case reloctype.R_METHODOFF:
					if linker.SymMap[loc.SymName].Kind == symkind.STEXT {
						addrBase = segment.codeBase
					}
					offset := int(symAddr) - addrBase
					if isOverflowInt32(offset) {
						if sym, ok := linker.SymMap[loc.SymName]; ok {
							offset = sym.Offset + loc.Add
						} else {
							err = fmt.Errorf("symName:%s relocateType:%s, offset:%d is overflow!\n", loc.SymName, reloctype.RelocTypeString(loc.Type), offset)
						}
					}
					byteOrder.PutUint32(relocByte[loc.Offset:], uint32(offset))
				case reloctype.R_GOTPCREL, reloctype.R_ARM64_GOTPCREL:
					offset := uint32(segment.dataBase + segment.dataOff - (addrBase + loc.GetEnd()))
					byteOrder.PutUint32(relocByte[loc.Offset:], offset)
					putAddressAddOffset(byteOrder, segment.dataByte, &segment.dataOff, uint64(symAddr))
				case reloctype.R_USETYPE,
					reloctype.R_USEIFACE,
					reloctype.R_USEIFACEMETHOD,
					reloctype.R_ADDRCUOFF,
					reloctype.R_INITORDER:
					//nothing todo
				case reloctype.R_KEEP:
					//nothing todo
				default:
					err = fmt.Errorf("unknown reloc type:%s sym:%s", reloctype.RelocTypeString(loc.Type), loc.SymName)
				}
			}
			if err != nil {
				return err
			}
		}
	}
	return err
}

func getUseIfaceTypeMap(symbol *obj.Sym) map[string]int {
	typMap := make(map[string]int, 0)
	for _, l := range symbol.Reloc {
		if l.Type == reloctype.R_USEIFACE || l.Type == reloctype.R_USEIFACEMETHOD {
			typMap[l.SymName] = 0x1
		}
	}
	return typMap
}

func isUseIfaceMethod(typMap map[string]int, reloc *obj.Reloc) bool {
	interTypeName, typeName := getTypeNameByItab(reloc.SymName)
	_, interExist := typMap[interTypeName]
	_, typExist := typMap[typeName]
	return interExist && typExist
}
