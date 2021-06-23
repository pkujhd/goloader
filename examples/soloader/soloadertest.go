package main

import (
	"fmt"
)

func throw() {
	panic("panic call throw function")
}

func inline() {
	throw()
}

func main() {
	fmt.Println("dynamic loader test done!")
	inline()
}
