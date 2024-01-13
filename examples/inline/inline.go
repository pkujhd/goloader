package inline

func throw() {
	panic("panic call function")
}

func inline() {
	throw()
}

func main() {
	inline()
}
