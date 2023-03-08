package p

import "fmt"

type Intf interface {
	Print(string)
}

type Stru struct {
}

func (Stru *Stru) Print(s string) {
	fmt.Println(s)
}
