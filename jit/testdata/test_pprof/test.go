package test_pprof

import (
	"fmt"
	"time"
)

func TestPprof() int {
	var m = map[string]int{}
	for i := 0; i < 100; i++ {
		m[fmt.Sprintf("%v", i)] = i
	}
	time.Sleep(time.Second / 1000)
	return m["1"]
}
