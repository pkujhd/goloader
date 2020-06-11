package main

import "fmt"

func addfloat64(a, b float64) float64 {
	if b < 0 {
		fmt.Println("addfloat64:Do not add negative number")
	}
	return a + b
}

func addfloat32(a, b float32) float32 {
	if b < 0 {
		fmt.Println("addfloat32:Do not add negative number")
	}
	return a + b
}

func addint64(a, b int64) int64 {
	if b < 0 {
		fmt.Println("addfloat32:Do not add negative number")
	}
	return a + b
}

func LoaderString(value interface{}) bool {
	fmt.Println("LoaderString:", fmt.Sprint(value))
	return false
}

func main() {
	LoaderString(0)
	LoaderString(1)

	f32_x := float32(1.0)
	f32_y := float32(2.0)
	f32_z := float32(-1.0)
	f32_val := float32(0.0)
	f32_val = addfloat32(f32_x, f32_z)
	fmt.Printf("f32 %f + %f = %f\n", f32_x, f32_z, f32_val)
	f32_val = addfloat32(f32_x, f32_y)
	fmt.Printf("f32 %f + %f = %f\n", f32_x, f32_y, f32_val)

	f64_x := float64(1.0)
	f64_y := float64(2.0)
	f64_z := float64(-1.0)
	f64_val := float64(0.0)
	f64_val = addfloat64(f64_x, f64_z)
	fmt.Printf("f64 %f + %f = %f\n", f64_x, f64_z, f64_val)
	f64_val = addfloat64(f64_x, f64_y)
	fmt.Printf("f64 %f + %f = %f\n", f64_x, f64_y, f64_val)

	i64_x := int64(1.0)
	i64_y := int64(2.0)
	i64_z := int64(-1.0)
	i64_val := int64(0.0)
	i64_val = addint64(i64_x, i64_z)
	fmt.Printf("i64 %d + %d = %d\n", i64_x, i64_z, i64_val)
	i64_val = addint64(i64_x, i64_y)
	fmt.Printf("i64 %d + %d = %d\n", i64_x, i64_y, i64_val)

}
