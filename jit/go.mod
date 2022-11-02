module github.com/pkujhd/goloader/jit

go 1.19

require (
	github.com/bmatcuk/doublestar/v4 v4.4.0
	github.com/pkujhd/goloader v0.0.0-20221026112716-fed0a9e75321
	github.com/pkujhd/goloader/jit/testdata v0.0.0-20221026112716-fed0a9e75321
)

require github.com/opentracing/opentracing-go v1.2.0 // indirect

replace github.com/pkujhd/goloader => ../

replace github.com/pkujhd/goloader/jit/testdata => ./testdata
