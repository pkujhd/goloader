package test_func

import (
	"math"
)

func AllTheMaxes(a, b float64) (float64, float64, float64, float64) {
	max1 := MyMax(a, b)
	max2 := archMax(a, b)
	max3 := math.Max(a, b)
	exp := math.Exp2(a)
	return max1, max2, max3, exp
}

func MyMax(x, y float64) float64 {
	return archMax(x, y)
}

func archMax(x, y float64) float64
