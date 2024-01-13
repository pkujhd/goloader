package gctest

import (
	"fmt"
	"runtime"
)

type Vertex struct {
	X, Y int
}

func (v *Vertex) Print() {
	fmt.Println("print", v)
}

type PrintInf interface {
	Print()
}

var uptr *Vertex
var uptra *Vertex
var strptr *string

func main() {
	uptr = new(Vertex)
	uptra = uptr
	uptr.X = 1000
	uptr.Y = 1000
	uptr = new(Vertex)
	fmt.Println(uptr, uptra)
	runtime.GC()
	runtime.GC()
	runtime.GC()
	fmt.Println(uptr.X, uptr.Y, uptra)
	var str = "I am a string"
	str = fmt.Sprintf("%s %d", str, uptra.X)
	strptr = &str
	fmt.Println(*strptr)
	runtime.GC()
	runtime.GC()
	runtime.GC()
	fmt.Println(*strptr)
}
