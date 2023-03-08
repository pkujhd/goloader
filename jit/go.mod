module github.com/eh-steve/goloader/jit

go 1.18

require (
	github.com/bmatcuk/doublestar/v4 v4.4.0
	github.com/eh-steve/goloader v0.0.0-20221026112716-fed0a9e75321
	github.com/eh-steve/goloader/jit/testdata v0.0.0-20221026112716-fed0a9e75321
	gonum.org/v1/gonum v0.12.0
)

require github.com/opentracing/opentracing-go v1.2.0 // indirect

replace github.com/eh-steve/goloader => ../

replace github.com/eh-steve/goloader/jit/testdata => ./testdata
