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
	armcode   = []byte{0x04, 0xF0, 0x1F, 0xE5} //LDR PC, [PC, #-4]
	arm64code = []byte{
		// X16 and X17 are the IP0 and IP1 intra-procedure-call corruptible registers -
		// since Go only uses them for the stack prologue and epilogue calculations,
		// and we should already be clear of that by the time we hit a R_CALLARM64,
		// so we should be able to safely use them for far jumps
		0x51, 0x00, 0x00, 0x58, // LDR X17 [PC+8] - read 64 bit address from PC+8 into X17
		0x20, 0x02, 0x1f, 0xd6, // BR  X17 - jump to address in X17
	}
	arm64Bcode = []byte{0x00, 0x00, 0x00, 0x14} // B [PC+0x0]
)

// x86/amd64
var (
	x86amd64JMPLcode        = []byte{0xff, 0x25, 0x00, 0x00, 0x00, 0x00} // JMPL *ADDRESS
	x86amd64JMPNearCode     = []byte{0xE9, 0x00, 0x00, 0x00, 0x00}       // JMP (PCREL offset)+4
	x86amd64replaceCALLcode = []byte{
		0x50,                                                             // PUSH RAX
		0x48, 0xb8, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // MOVABS RAX x (64 bit)
		0xff, 0xd0, // CALL RAX
		0x58, // POP RAX
	}
	x86amd64replaceCMPLcode = []byte{
		0x50,                                     // PUSH EAX
		0x53,                                     // PUSH EBX
		0x48, 0x8b, 0x05, 0x0f, 0x00, 0x00, 0x00, // MOVE EAX x
		0x48, 0x8b, 0x18, // MOVE EBX [EAX]
		0x48, 0x83, 0xfb, 0x00, // CMPL EBX x(8bits)
		0x5b,                               // POP EBX
		0x58,                               // POP EAX
		0xff, 0x25, 0x08, 0x00, 0x00, 0x00} // JMPL *ADDRESS
	x86amd64replaceMOVQcode = []byte{
		0x48, 0xb8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //MOVE RxX x
		0xff, 0x25, 0x00, 0x00, 0x00, 0x00} //JMPL *ADDRESS
)
