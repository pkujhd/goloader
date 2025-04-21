//go:build 386 || amd64
// +build 386 amd64

package obj

import (
	"cmd/objfile/sys"
	"fmt"

	_ "unsafe"
)

//see:src/cmd/vendor/golang.org/x/arch/x86/x86asm
type Op uint32

// Prefixes is an array of prefixes associated with a single instruction.
// The prefixes are listed in the same order as found in the instruction:
// each prefix byte corresponds to one slot in the array. The first zero
// in the array marks the end of the prefixes.
type Prefixes [14]Prefix

// A Prefix represents an Intel instruction prefix.
// The low 8 bits are the actual prefix byte encoding,
// and the top 8 bits contain distinguishing bits and metadata.
type Prefix uint16

// An Args holds the instruction arguments.
// If an instruction has fewer than 4 arguments,
// the final elements in the array are nil.
type Args [4]Arg

// An Arg is a single instruction argument,
// one of these types: Reg, Mem, Imm, Rel.
type Arg interface {
	String() string
	isArg()
}

// An Inst is a single instruction.
type Inst struct {
	Prefix   Prefixes // Prefixes applied to the instruction.
	Op       Op       // Opcode mnemonic
	Opcode   uint32   // Encoded opcode bits, left aligned (first byte is Opcode>>24, etc)
	Args     Args     // Instruction arguments, in Intel order
	Mode     int      // processor mode in bits: 16, 32, or 64
	AddrSize int      // address size in bits: 16, 32, or 64
	DataSize int      // operand size in bits: 16, 32, or 64
	MemBytes int      // size of memory argument in bytes: 1, 2, 4, 8, 16, and so on.
	Len      int      // length of encoded instruction in bytes
	PCRel    int      // length of PC-relative address in instruction encoding
	PCRelOff int      // index of start of PC-relative address in instruction encoding
}

//go:linkname Decode cmd/vendor/golang.org/x/arch/x86/x86asm.Decode
func Decode(src []byte, mode int) (inst Inst, err error)

//go:linkname OpString cmd/vendor/golang.org/x/arch/x86/x86asm.Op.String
func OpString(op Op) string

func (op Op) String() string { return OpString(op) }

func MarkReloc(text []byte, relocs []Reloc, offset int, archName string) {
	pc := 0
	relocId := 0
	arch := 32
	if archName == sys.ArchAMD64.Name {
		arch = 64
	}
	relocsLen := len(relocs)
	if relocsLen == 0 {
		return
	}
	textLen := len(text)
	for textLen != pc && relocId < relocsLen {
		inst, err := Decode(text[pc:], arch)
		if err != nil || inst.Len == 0 || inst.Op == 0 {
			pc = pc + 1
		} else {
			for relocId < relocsLen && relocs[relocId].Offset >= pc && relocs[relocId].Offset < pc+inst.Len {
				relocs[relocId].Op = uint(inst.Op)
				relocs[relocId].Text = text[pc : pc+inst.Len]
				relocs[relocId].Start = pc + offset
				relocs[relocId].End = pc + offset + inst.Len
				for _, arg := range inst.Args {
					if arg != nil {
						relocs[relocId].Args = append(relocs[relocId].Args, arg.String())
					}
				}
				relocId++
			}
			pc = pc + inst.Len
		}
	}
}

func GetOpName(op uint) string {
	return Op(op).String()
}

func DumpCode(text []byte, archName string) {
	pc := 0
	textLen := len(text)
	arch := 32
	if archName == sys.ArchAMD64.Name {
		arch = 64
	}
	for textLen != pc {
		inst, err := Decode(text[pc:], arch)
		if err != nil {
			fmt.Println("x86asm.Decode failed")
		}
		fmt.Println(inst)
		pc = pc + inst.Len
	}
}

func IsExtraRegister(regName string) bool {
	switch regName {
	case "R8", "R9", "R10", "R11", "R12", "R13", "R14", "R15":
		return true
	default:
		return false
	}
}
