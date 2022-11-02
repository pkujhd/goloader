package test_func

import (
	"gonum.org/v1/gonum/mat"
)

func MatPow() {
	m := mat.NewDense(5, 5, make([]float64, 5*5))
	p := mat.NewDense(5, 5, make([]float64, 5*5))
	m.Pow(p, 2)
}
