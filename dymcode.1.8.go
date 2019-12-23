// +build go1.8 go1.9 go1.10 go1.11
// +build !go1.12,!go1.13

package goloader

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"unsafe"
)

func Load(code *CodeReloc, symPtr map[string]uintptr) (*CodeModule, error) {
	pCodeLen := len(code.Code) + len(code.Data)
	codeLen := int(float32(pCodeLen) * 1.5)
	codeByte, err := Mmap(codeLen)
	if err != nil {
		return nil, err
	}

	var codeModule = CodeModule{
		Syms:    make(map[string]uintptr),
		typemap: make(map[typeOff]uintptr),
	}
	var errBuf bytes.Buffer

	base := int((*sliceHeader)(unsafe.Pointer(&codeByte)).Data)
	dataBase := base + len(code.Code)

	var symAddrs = make([]int, len(code.Syms))
	var itabIndexs []int
	var funcTypeMap = make(map[string]*int)
	for i, sym := range code.Syms {
		if sym.Offset == -1 {
			if ptr, ok := symPtr[sym.Name]; ok {
				symAddrs[i] = int(ptr)
			} else {
				symAddrs[i] = -1
				strWrite(&errBuf, "unresolve external:", sym.Name, "\n")
			}
		} else if sym.Name == TLSNAME {
			RegTLS(symPtr, sym.Offset)
		} else if sym.Kind == STEXT {
			symAddrs[i] = code.Syms[i].Offset + base
			codeModule.Syms[sym.Name] = uintptr(symAddrs[i])
		} else if strings.HasPrefix(sym.Name, "go.itab") {
			if ptr, ok := symPtr[sym.Name]; ok {
				symAddrs[i] = int(ptr)
			} else {
				itabIndexs = append(itabIndexs, i)
			}
		} else {
			symAddrs[i] = code.Syms[i].Offset + dataBase

			if strings.HasPrefix(sym.Name, "type.func") {
				funcTypeMap[sym.Name] = &symAddrs[i]
			}
			if strings.HasPrefix(sym.Name, "type.") {
				if ptr, ok := symPtr[sym.Name]; ok {
					symAddrs[i] = int(ptr)
				}
			}
		}
	}

	var itabSymMap = make(map[string]int)
	for _, itabIndex := range itabIndexs {
		curSym := code.Syms[itabIndex]
		sym1 := symAddrs[curSym.Reloc[0].SymOff]
		sym2 := symAddrs[curSym.Reloc[1].SymOff]
		itabSymMap[curSym.Name] = len(codeModule.itabSyms)
		codeModule.itabSyms = append(codeModule.itabSyms, itabSym{inter: sym1, _type: sym2})

		if sym1 == -1 || sym2 == -1 {
			continue
		}
		addIFaceSubFuncType(funcTypeMap, codeModule.typemap,
			(*interfacetype)(unsafe.Pointer(uintptr(sym1))), base)
	}

	var armcode = []byte{0x04, 0xF0, 0x1F, 0xE5, 0x00, 0x00, 0x00, 0x00}
	var arm64code = []byte{0x43, 0x00, 0x00, 0x58, 0x60, 0x00, 0x1F, 0xD6, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	var x86code = []byte{0xff, 0x25, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	var movcode byte = 0x8b
	var leacode byte = 0x8d
	var cmplcode byte = 0x83
	var jmpcode byte = 0xe9
	var jmpOff = pCodeLen
	for _, curSym := range code.Syms {
		for _, loc := range curSym.Reloc {
			sym := code.Syms[loc.SymOff]
			if symAddrs[loc.SymOff] == -1 {
				continue
			}
			if symAddrs[loc.SymOff] == 0 && strings.HasPrefix(sym.Name, "go.itab") {
				codeModule.itabs = append(codeModule.itabs,
					itabReloc{locOff: loc.Offset, symOff: itabSymMap[sym.Name],
						size: loc.Size, locType: loc.Type, add: loc.Add})
				continue
			}

			var offset int
			switch loc.Type {
			case R_TLS_LE:
				binary.LittleEndian.PutUint32(code.Code[loc.Offset:], uint32(symPtr[TLSNAME]))
				continue
			case R_CALL, R_PCREL:
				var relocByte = code.Data
				var addrBase = dataBase
				if curSym.Kind == STEXT {
					addrBase = base
					relocByte = code.Code
				}
				offset = symAddrs[loc.SymOff] - (addrBase + loc.Offset + loc.Size) + loc.Add
				if offset > 0x7fffffff || offset < -0x8000000 {
					if jmpOff+8 > codeLen {
						strWrite(&errBuf, "len overflow", "sym:", sym.Name, "\n")
						continue
					}
					rb := relocByte[loc.Offset-2:]
					if loc.Type == R_CALL {
						offset = (base + jmpOff) - (addrBase + loc.Offset + loc.Size)
						copy(codeByte[jmpOff:], x86code)
						binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
						if uint64(symAddrs[loc.SymOff]+loc.Add) > 0xFFFFFFFF {
							binary.LittleEndian.PutUint64(codeByte[jmpOff+6:], uint64(symAddrs[loc.SymOff]+loc.Add))
						} else {
							binary.LittleEndian.PutUint32(codeByte[jmpOff+6:], uint32(symAddrs[loc.SymOff]+loc.Add))
						}
						jmpOff += len(x86code)
					} else if rb[0] == leacode || rb[0] == movcode || rb[0] == cmplcode || rb[1] == jmpcode {
						offset = (base + jmpOff) - (addrBase + loc.Offset + loc.Size)
						binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
						if rb[0] == leacode {
							rb[0] = movcode
						}
						if uint64(symAddrs[loc.SymOff]+loc.Add) > 0xFFFFFFFF {
							binary.LittleEndian.PutUint64(codeByte[jmpOff:], uint64(symAddrs[loc.SymOff]+loc.Add))
							jmpOff += 12
						} else {
							binary.LittleEndian.PutUint32(codeByte[jmpOff:], uint32(symAddrs[loc.SymOff]+loc.Add))
							jmpOff += 8
						}
					} else {
						strWrite(&errBuf, "offset overflow sym:", sym.Name, "\n")
						binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
					}
					continue
				}
				binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
			case R_CALLARM, R_CALLARM64:
				var add = loc.Add
				var pcOff = 0
				if loc.Type == R_CALLARM {
					add = loc.Add & 0xffffff
					if add > 256 {
						add = 0
					} else {
						add += 2
					}
					pcOff = 8
				}
				offset = (symAddrs[loc.SymOff] - (base + loc.Offset + pcOff) + add) / 4
				if offset > 0x7FFFFF || offset < -0x800000 {
					if jmpOff+4 > codeLen {
						strWrite(&errBuf, "len overflow", "sym:", sym.Name, "\n")
						continue
					}
					align := jmpOff % 4
					if align != 0 {
						jmpOff += (4 - align)
					}
					offset = (jmpOff - (loc.Offset + pcOff)) / 4
					var v = uint32(offset)
					b := code.Code[loc.Offset:]
					b[0] = byte(v)
					b[1] = byte(v >> 8)
					b[2] = byte(v >> 16)
					var jmpLocOff = 0
					var jmpLen = 0
					if loc.Type == R_CALLARM64 {
						copy(codeByte[jmpOff:], arm64code)
						jmpLen = len(arm64code)
						jmpLocOff = 8
					} else {
						copy(codeByte[jmpOff:], armcode)
						jmpLen = len(armcode)
						jmpLocOff = 4
					}
					*(*uintptr)(unsafe.Pointer(&(codeByte[jmpOff+jmpLocOff:][0]))) = uintptr(symAddrs[loc.SymOff] + add*4)
					jmpOff += jmpLen
					continue
				}
				var v = uint32(offset)
				b := code.Code[loc.Offset:]
				b[0] = byte(v)
				b[1] = byte(v >> 8)
				b[2] = byte(v >> 16)
			case R_ADDRARM64:
				if curSym.Kind != STEXT {
					strWrite(&errBuf, "not in code?\n")
				}
				relocADRP(code.Code[loc.Offset:], base+loc.Offset, symAddrs[loc.SymOff], sym.Name)
			case R_ADDR:
				var relocByte = code.Data
				if curSym.Kind == STEXT {
					relocByte = code.Code
				}
				offset = symAddrs[loc.SymOff] + loc.Add
				*(*uintptr)(unsafe.Pointer(&(relocByte[loc.Offset:][0]))) = uintptr(offset)
			case R_CALLIND:

			case R_ADDROFF, R_WEAKADDROFF, R_METHODOFF:
				var relocByte = code.Data
				var addrBase = base
				if curSym.Kind == STEXT {
					strWrite(&errBuf, "impossible!", sym.Name, "locate on code segment", "\n")
				}
				offset = symAddrs[loc.SymOff] - addrBase + loc.Add
				binary.LittleEndian.PutUint32(relocByte[loc.Offset:], uint32(offset))
			default:
				strWrite(&errBuf, "unknown reloc type:", strconv.Itoa(loc.Type), sym.Name, "\n")
			}

		}
	}

	var module moduledata
	module.ftab = make([]functab, len(code.Mod.ftab))
	copy(module.ftab, code.Mod.ftab)
	pclnOff := len(code.Mod.pclntable)
	module.pclntable = make([]byte, len(code.Mod.pclntable)+
		(_funcSize+256)*len(code.Mod.ftab))
	copy(module.pclntable, code.Mod.pclntable)
	module.findfunctab = (uintptr)(unsafe.Pointer(&code.Mod.pcfunc[0]))
	module.minpc = (uintptr)(unsafe.Pointer(&codeByte[0]))
	module.maxpc = (uintptr)(unsafe.Pointer(&codeByte[len(code.Code)-1])) + 2
	module.filetab = code.Mod.filetab
	module.typemap = codeModule.typemap
	module.types = uintptr(base)
	module.etypes = uintptr(base + codeLen)
	module.text = uintptr(base)
	module.etext = uintptr(base + len(code.Code))
	codeModule.pcfuncdata = code.Mod.pcfunc // hold reference
	codeModule.stkmaps = code.Mod.stkmaps
	for i := range module.ftab {
		module.ftab[i].entry = uintptr(symAddrs[int(code.Mod.ftab[i].entry)])

		ptr2 := (uintptr)(unsafe.Pointer(&module.pclntable[pclnOff]))
		if PtrSize == 8 && ptr2&4 != 0 {
			pclnOff += 4
		}
		module.ftab[i].funcoff = uintptr(pclnOff)
		fi := code.Mod.funcinfo[i]
		fi.entry = module.ftab[i].entry
		copy2Slice(module.pclntable[pclnOff:],
			unsafe.Pointer(&fi._func), _funcSize)
		pclnOff += _funcSize

		if len(fi.pcdata) > 0 {
			size := int(4 * fi.npcdata)
			copy2Slice(module.pclntable[pclnOff:],
				unsafe.Pointer(&fi.pcdata[0]), size)
			pclnOff += size
		}

		var funcdata = make([]uintptr, len(fi.funcdata))
		copy(funcdata, fi.funcdata)
		for i, v := range funcdata {
			if v != 0 {
				funcdata[i] = (uintptr)(unsafe.Pointer(&(code.Mod.stkmaps[v][0])))
			} else {
				funcdata[i] = (uintptr)(0)
			}
		}
		ptr := (uintptr)(unsafe.Pointer(&module.pclntable[pclnOff-1])) + 1
		if PtrSize == 8 && ptr&4 != 0 {
			t := [4]byte{}
			copy(module.pclntable[pclnOff:], t[:])
			pclnOff += len(t)
		}
		funcDataSize := int(PtrSize * fi.nfuncdata)
		copy2Slice(module.pclntable[pclnOff:],
			unsafe.Pointer(&funcdata[0]), funcDataSize)
		pclnOff += funcDataSize

	}
	module.pclntable = module.pclntable[:pclnOff]
	module.ftab = append(module.ftab, functab{})
	for i := len(module.ftab) - 1; i > 0; i-- {
		module.ftab[i] = module.ftab[i-1]
	}
	module.ftab = append(module.ftab, functab{})
	module.ftab[0].entry = module.minpc
	module.ftab[len(module.ftab)-1].entry = module.maxpc

	modulesLock.Lock()
	addModule(&codeModule, &module)
	modulesLock.Unlock()

	copy(codeByte, code.Code)
	copy(codeByte[len(code.Code):], code.Data)
	codeModule.CodeByte = codeByte

	for i := range codeModule.itabSyms {
		it := &codeModule.itabSyms[i]
		if it.inter == -1 || it._type == -1 {
			continue
		}
		it.ptr = getitab(it.inter, it._type, false)
	}
	for _, it := range codeModule.itabs {
		symAddr := codeModule.itabSyms[it.symOff].ptr
		if symAddr == 0 {
			continue
		}
		switch it.locType {
		case R_PCREL:
			pc := base + it.locOff + it.size
			offset := symAddr - pc + it.add
			if offset > 0x7FFFFFFF || offset < -0x80000000 {
				offset = (base + jmpOff) - pc + it.add
				binary.LittleEndian.PutUint32(codeByte[it.locOff:], uint32(offset))
				codeByte[it.locOff-2:][0] = movcode
				*(*uintptr)(unsafe.Pointer(&(codeByte[jmpOff:][0]))) = uintptr(symAddr)
				jmpOff += PtrSize
				continue
			}
			binary.LittleEndian.PutUint32(codeByte[it.locOff:], uint32(offset))
		case R_ADDRARM64:
			relocADRP(codeByte[it.locOff:], base+it.locOff, symAddr, "unknown")
		}
	}

	if errBuf.Len() > 0 {
		return &codeModule, errors.New(errBuf.String())
	}
	return &codeModule, nil
}
