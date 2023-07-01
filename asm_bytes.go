package goloader

const (
	x86amd64MOVcode  byte = 0x8B
	x86amd64LEAcode  byte = 0x8D
	x86amd64CMPLcode byte = 0x83
	x86amd64CALLcode byte = 0xE8
	x86amd64JMPcode  byte = 0xE9
)

// arm/arm64
var (
	armReplaceCallCode = []byte{0x04, 0xF0, 0x1F, 0xE5} //LDR PC, [PC, #-4]
	// register X27 reserved for liblink. see:^src/cmd/objfile/obj/arm64/a.out.go
	arm64ReplaceCALLCode = []byte{
		0x5B, 0x00, 0x00, 0x58, // LDR X27 [PC+8] - read 64 bit address from PC+8 into X27
		0x60, 0x03, 0x1F, 0xD6, // BR  X27 - jump to address in X27
	}
	arm64Bcode   = []byte{0x00, 0x00, 0x00, 0x14} // B [PC+0x0]
	arm64LDRcode = []byte{0x00, 0x00, 0x40, 0xF9} // LDR XX [XX]
	arm64Nopcode = []byte{0x1f, 0x20, 0x03, 0xd5} // NOP
)

// x86/amd64
var (
	x86amd64NOPcode         = []byte{0x90}                               // NOP
	x86amd64JMPLcode        = []byte{0xff, 0x25, 0x00, 0x00, 0x00, 0x00} // JMPL *ADDRESS
	x86amd64JMPNcode        = []byte{0xE9, 0x00, 0x00, 0x00, 0x00}       // JMP (PCREL offset)+4
	x86amd64replaceCMPLcode = []byte{
		0x50,                                                       // PUSH RAX
		0x48, 0xb8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // MOVABS RAX x (64 bit)
		0x48, 0x83, 0x38, 0x00, // CMPL [RAX] x(8bits)
		0x58, // POP RAX
	}
	x86amd64replaceMOVQcode = []byte{
		0x48, 0xb8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // MOVABS RxX x (64 bit)
		0x48, 0x8b, 0x00, // MOV RxX [RxX]
	}
)
