module github.com/eh-steve/goloader/jit

go 1.18

require (
	github.com/bmatcuk/doublestar/v4 v4.4.0
	github.com/eh-steve/goloader v0.0.0-20230413075038-99bc3d306b6f
	github.com/eh-steve/goloader/jit/testdata v0.0.0-20230413075038-99bc3d306b6f
	github.com/eh-steve/goloader/unload v0.0.0-20230413075038-99bc3d306b6f
)

require github.com/opentracing/opentracing-go v1.2.0 // indirect

replace github.com/eh-steve/goloader => ../

replace github.com/eh-steve/goloader/jit/testdata => ./testdata

replace github.com/eh-steve/goloader/unload => ../unload
