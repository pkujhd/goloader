package test_cgo

/*
#define SOME_CPP_CONSTANT 5

int mul(int a, int b ) {
	return a * b;
}
int add(int a, int b ) {
	return a * b;
}
*/
import "C"

func CGoCall(a, b int32) (int32, int32, int32) {
	mul := int32(C.mul(C.int(a), C.int(b)))
	add := int32(C.add(C.int(a), C.int(b)))
	constant := int32(C.SOME_CPP_CONSTANT)
	return mul, add, constant
}
