package test_cgo_package

/*
#include "test.h"
*/
import "C"

func CGoCall(a, b int32) (int32, int32, int32, int32, int32) {
	mul := int32(C.mul(C.int(a), C.int(b)))
	add := int32(C.add(C.int(a), C.int(b)))
	constant := int32(C.SOME_CPP_CONSTANT)
	cCallsGo := int32(C.Cgomul(C.mul(C.int(a), C.int(b)), C.add(C.int(a), C.int(b))))
	C.blah += C.int(constant)
	return mul, add, constant, int32(C.blah), cCallsGo
}

//export goMul
func goMul(a, b C.int) C.int {
	return a * b
}
