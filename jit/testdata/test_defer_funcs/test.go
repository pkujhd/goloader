package test_defer_funcs

import (
	"gonum.org/v1/gonum/mat"
	"math/rand"
)

func TestOpenDefer() {
	solveMatrix()
}

func solveMatrix() {
	a := mat.NewDense(5, 5, make([]float64, 25))
	b := mat.NewDense(5, 5, make([]float64, 25))
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			a.Set(i, j, rand.Float64())
			b.Set(i, j, rand.Float64())
		}
	}
	c := mat.NewDense(5, 5, make([]float64, 25))
	err := c.Solve(a, b)
	if err != nil {
		panic(err)
	}
}
