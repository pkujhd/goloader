package test_embedded

type someStruct1 struct {
}

type someStruct2 struct {
	*someStruct1
}

type someInterface interface {
	method1(i int) int
	method2()
}

func (s *someStruct1) method1(i int) int {
	return i
}

func (s *someStruct2) method2() {

}

var _ someInterface = &someStruct2{}

func DoIt(i someInterface) int {
	return i.method1(5)
}

func MakeIt() int {
	var s someInterface = &someStruct2{}
	return DoIt(s)
}
