package test_issue78

var val = 1

func Test() (output int) {
	val++
	return val
}
