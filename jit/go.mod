module github.com/eh-steve/goloader/jit

go 1.18

require (
	github.com/eh-steve/goloader v0.0.0-20230531015719-87f559aca3b2
	github.com/eh-steve/goloader/jit/testdata v0.0.0-20230531015719-87f559aca3b2
	github.com/eh-steve/goloader/unload v0.0.0-20230531015719-87f559aca3b2
)

require github.com/opentracing/opentracing-go v1.2.0 // indirect

//replace github.com/eh-steve/goloader => ../
