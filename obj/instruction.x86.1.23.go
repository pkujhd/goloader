//go:build (386 || amd64) && go1.23 && !go1.27
// +build 386 amd64
// +build go1.23
// +build !go1.27

package obj

import "unsafe"

//see:src/cmd/vendor/golang.org/x/arch/x86/x86asm

// An Args holds the instruction arguments.
// If an instruction has fewer than 4 arguments,
// the final elements in the array are nil.
type Args [4]Arg

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

func Decode(src []byte, mode int) (inst Inst, err error) {
	return decode1(src, mode, false)
}

var (
	decode1  func(src []byte, mode int, gnuCompat bool) (Inst, error) = nil
	OpString func(op Op) string                                       = nil
)

func AddInstLinkName(symPtr map[string]uintptr) {
	//prevent the golang compiler from pruning x86asm's symbols
	_Dummy(false)

	*(*uintptr)(unsafe.Pointer(&decode1)) = getFuncPointer(symPtr, "cmd/vendor/golang.org/x/arch/x86/x86asm.decode1")
	*(*uintptr)(unsafe.Pointer(&OpString)) = getFuncPointer(symPtr, "cmd/vendor/golang.org/x/arch/x86/x86asm.Op.String")
}
