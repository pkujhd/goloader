package goloader

import (
	"fmt"
	"strings"

	"github.com/pkujhd/goloader/obj"
	"github.com/pkujhd/goloader/objabi/reloctype"
	"github.com/pkujhd/goloader/objabi/symkind"
	"github.com/pkujhd/goloader/objabi/tls"
)

func (linker *Linker) relocateADRP(mCode []byte, loc obj.Reloc, segment *segment, symAddr uintptr) (err error) {
	byteorder := linker.Arch.ByteOrder
	offset := int64(symAddr) + int64(loc.Add) - ((int64(segment.codeBase) + int64(loc.Offset)) &^ 0xFFF)
	//overflow
	if offset >= 1<<32 || offset < -1<<32 {
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
			blcode := byteorder.Uint32(arm64BLcode)
			blcode |= ((uint32(segment.codeOff) - uint32(loc.Offset)) >> 2) & 0x01FFFFFF
			if segment.codeOff-loc.Offset < 0 {
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
			putAddressAddOffset(byteorder, segment.codeByte, &segment.codeOff, uint64(llow)|(uint64(lhigh)<<32))
			hlow = ((addr & 0x1F) | hlow) | uint32(((uint64(symAddr)>>32)&0xFFFF)<<5)
			hhigh = ((addr & 0x1F) | hhigh) | uint32((uint64(symAddr)>>48)<<5)
			putAddressAddOffset(byteorder, segment.codeByte, &segment.codeOff, uint64(hlow)|(uint64(hhigh)<<32))
			if loc.Type != reloctype.R_ADDRARM64 {
				//LDR
				ldrcode := byteorder.Uint32(arm64LDRcode) | addr&0x1F | ((addr & 0x1F) << 5)
				byteorder.PutUint32(segment.codeByte[segment.codeOff:], ldrcode)
				segment.codeOff += Uint32Size
			}
			blcode = byteorder.Uint32(arm64BLcode)
			blcode |= ((uint32(loc.Offset) - uint32(segment.codeOff) + PtrSize) >> 2) & 0x01FFFFFF
			if loc.Offset-segment.codeOff+PtrSize < 0 {
				blcode |= 0x02000000
			}
			byteorder.PutUint32(segment.codeByte[segment.codeOff:], blcode)
			segment.codeOff += Uint32Size
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
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		offset = (segment.codeBase + segment.codeOff) - (addrBase + loc.Offset + loc.Size)
		copy(segment.codeByte[segment.codeOff:], x86amd64JMPLcode)
		segment.codeOff += len(x86amd64JMPLcode)
		putAddressAddOffset(byteorder, segment.codeByte, &segment.codeOff, uint64(addr)+uint64(loc.Add))
	}
	byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
}

func (linker *Linker) relocatePCREL(addr uintptr, loc obj.Reloc, segment *segment, relocByte []byte, addrBase int) (err error) {
	byteorder := linker.Arch.ByteOrder
	offset := int(addr) - (addrBase + loc.Offset + loc.Size) + loc.Add
	if offset > 0x7FFFFFFF || offset < -0x80000000 {
		offset = (segment.codeBase + segment.codeOff) - (addrBase + loc.Offset + loc.Size)
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
		} else {
			return fmt.Errorf("not support code:%v!\n", relocByte[loc.Offset-2:loc.Offset])
		}
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
		if opcode == x86amd64CMPLcode || opcode == x86amd64MOVcode {
			putAddressAddOffset(byteorder, segment.codeByte, &segment.codeOff, uint64(segment.codeBase+segment.codeOff+PtrSize))
			if opcode == x86amd64CMPLcode {
				copy(segment.codeByte[segment.codeOff:], x86amd64replaceCMPLcode)
				segment.codeByte[segment.codeOff+0x0F] = relocByte[loc.Offset+loc.Size]
				segment.codeOff += len(x86amd64replaceCMPLcode)
				putAddressAddOffset(byteorder, segment.codeByte, &segment.codeOff, uint64(addr))
			} else {
				copy(segment.codeByte[segment.codeOff:], x86amd64replaceMOVQcode)
				segment.codeByte[segment.codeOff+1] = regsiter
				copy2Slice(segment.codeByte[segment.codeOff+2:], addr, PtrSize)
				segment.codeOff += len(x86amd64replaceMOVQcode)
			}
			putAddressAddOffset(byteorder, segment.codeByte, &segment.codeOff, uint64(addrBase+loc.Offset+loc.Size-loc.Add))
		} else {
			putAddressAddOffset(byteorder, segment.codeByte, &segment.codeOff, uint64(addr))
		}
	} else {
		byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
	}
	return err
}

func (linker *Linker) relocteCALLARM(addr uintptr, loc obj.Reloc, segment *segment) {
	byteorder := linker.Arch.ByteOrder
	add := loc.Add
	if loc.Type == reloctype.R_CALLARM {
		add = int(signext24(int64(loc.Add&0xFFFFFF)) * 4)
	}
	offset := (int(addr) + add - (segment.codeBase + loc.Offset)) / 4
	if offset > 0x7FFFFF || offset < -0x800000 {
		segment.codeOff = alignof(segment.codeOff, PtrSize)
		off := uint32(segment.codeOff-loc.Offset) / 4
		if loc.Type == reloctype.R_CALLARM {
			add = int(signext24(int64(loc.Add&0xFFFFFF)+2) * 4)
			off = uint32(segment.codeOff-loc.Offset-8) / 4
		}
		putUint24(segment.codeByte[loc.Offset:], off)
		if loc.Type == reloctype.R_CALLARM64 {
			copy(segment.codeByte[segment.codeOff:], arm64code)
			segment.codeOff += len(arm64code)
		} else {
			copy(segment.codeByte[segment.codeOff:], armcode)
			segment.codeOff += len(armcode)
		}
		putAddressAddOffset(byteorder, segment.codeByte, &segment.codeOff, uint64(int(addr)+add))
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
	for _, symbol := range linker.symMap {
		// if the symbol exist in symPtr, so it doesnot need to relocate, the loader use the symbol in loader.
		if _, ok := symPtr[symbol.Name]; !ok || strings.HasPrefix(symbol.Name, MainPkgPrefix) {
			for _, loc := range symbol.Reloc {
				addr := symbolMap[loc.Sym.Name]
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
						if offset > 0x7FFFFFFF || offset < -0x80000000 {
							err = fmt.Errorf("symName:%s offset:%d is overflow!\n", sym.Name, offset)
						}
						byteorder.PutUint32(relocByte[loc.Offset:], uint32(offset))
					case reloctype.R_METHODOFF:
						if loc.Sym.Kind == symkind.STEXT {
							addrBase = segment.codeBase
						}
						offset := int(addr) - addrBase + loc.Add
						if offset > 0x7FFFFFFF || offset < -0x80000000 {
							err = fmt.Errorf("symName:%s offset:%d is overflow!\n", sym.Name, offset)
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
					default:
						err = fmt.Errorf("unknown reloc type:%d sym:%s", loc.Type, sym.Name)
					}
				}
				if err != nil {
					return err
				}
			}
		}
	}
	return err
}
