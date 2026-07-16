package reloctype

//go:inline
func IsDirectCall(r int) bool {
	switch r {
	case R_CALL, R_CALLARM, R_CALLARM64:
		return true
	default:
		return false
	}
}

//go:inline
func IsOffType(r int) bool {
	switch r {
	case R_ADDROFF, R_WEAKADDROFF, R_METHODOFF:
		return true
	default:
		return false
	}
}
