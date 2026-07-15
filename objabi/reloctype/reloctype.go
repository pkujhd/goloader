package reloctype

func IsDirectCall(r int) bool {
	switch r {
	case R_CALL, R_CALLARM, R_CALLARM64:
		return true
	}
	return false
}
