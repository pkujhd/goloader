//go:build (386 || amd64) && go1.27 && !go1.28
// +build 386 amd64
// +build go1.27
// +build !go1.28

package obj

//see:src/cmd/vendor/golang.org/x/arch/x86/x86asm

// An Args holds the instruction arguments.
// If an instruction has fewer than 4 arguments,
// the final elements in the array are nil.
type Args [6]Arg

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
	// AVX-512 flags
	Broadcast bool // EVEX broadcast
	Zeroing   bool // EVEX zeroing
	SAE       bool // Suppress All Exceptions
	Rounding  int8 // Rounding control (0-3), valid only when SAE is true
}
