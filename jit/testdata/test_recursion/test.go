package test_recursion

func Fib(n uint64) uint64 {
	if n <= 1 {
		return 1
	} else {
		return Fib(n-1) + Fib(n-2)
	}
}
