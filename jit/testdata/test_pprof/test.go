package test_pprof

import (
	"fmt"
	"time"
)

func TestPprofIssue75() int {
	var m = map[string]int{}
	for i := 0; i < 100; i++ {
		m[fmt.Sprintf("%v", i)] = i
	}
	time.Sleep(time.Microsecond * 200)
	return m["1"]
}
