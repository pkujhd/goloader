module github.com/eh-steve/goloader/jit

go 1.18

require (
	github.com/eh-steve/goloader v0.0.0-20230701143140-2238aec4d01c
	github.com/eh-steve/goloader/jit/testdata v0.0.0-20230701143140-2238aec4d01c
	github.com/eh-steve/goloader/unload v0.0.0-20230701143140-2238aec4d01c
)

require github.com/opentracing/opentracing-go v1.2.0 // indirect

replace github.com/eh-steve/goloader => ../
//replace github.com/eh-steve/goloader/jit/testdata => ./testdata
