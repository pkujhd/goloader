package test_init

import (
	"fmt"
)

var myMap = map[string]map[int]float64{
	"blah": {
		5: 6,
		7: 8,
	},
	"blah_blah": {
		1: 2,
		3: 4,
	},
}

func PrintMap() string {
	return fmt.Sprintf("%v", myMap)
}
