
# Goloader/JIT

![Build Status](https://github.com/pkujhd/goloader/workflows/goloader%20Testing/badge.svg)

Goloader can load and run Golang code at runtime.

The `goloader/jit` package can compile and load Go code from text, file or folder (including code with package imports).

Forked from **https://github.com/dearplain/goloader**, Take over maintenance because the original author is not in maintenance

## How does it work?

Goloader works like a linker: it relocates the address of symbols in an object file, generates runnable code, and then reuses the runtime function and the type pointer of the loader.

Goloader provides some information to the runtime and gc of Go, which allows it to work correctly with them.

Please note that Goloader is not a scripting engine. It reads the output of Go compiler and makes them runnable. All features of Go are supported, and run just as fast and lightweight as native Go code.

## Comparison with plugin

Goloader reuses the Go runtime, which makes it much smaller. And code loaded by Goloader is unloadable.

Goloader supports pprof tool(Yes, you can see code loaded by Goloader in pprof).

## OS/Arch Compatibility
JIT compiler tested/passing on:

| **OS/Arch** | amd64/+CGo         | arm64/+CGo          | amd64/-CGo         | arm64/-CGo         |
|-------------|--------------------|---------------------|--------------------|--------------------|
| Linux       | :heavy_check_mark: | :heavy_check_mark:  | :heavy_check_mark: | :heavy_check_mark: |
| Darwin      | :heavy_check_mark: | :heavy_check_mark:  | partial            | :x:                |
| Windows     | :x:                | :interrobang:       | :heavy_check_mark: | :interrobang:      |

## Build

**Make sure you're using go >= 1.18.**

First, execute the following command, then do build and test. This is because Goloader relies on the internal package, which is forbidden by the Go compiler.
```
cp -r $GOROOT/src/cmd/internal $GOROOT/src/cmd/objfile
```

## JIT compiler 

```go
package main

import (
	"fmt"
	"github.com/pkujhd/goloader/jit"
)

func main() {
	conf := jit.BuildConfig{
		DebugLog:    false,
		HeapStrings: true,
	}
	loadable, err := jit.BuildGoText(conf, `
package mypackage

import "encoding/json"

func MyFunc(input []byte) (interface{}, error) {
	var output interface{}
	err := json.Unmarshal(input, &output)
	return output, err
}
`)

	if err != nil {
		panic(err)
	}
	m, funcs, err := loadable.Load()
	if err != nil {
		panic(err)
	}
	defer m.Unload()

	f := funcs["MyFunc"].(func(input []byte) (interface{}, error))
	result, err := f([]byte(`{"test": "value"}`))
	if err != nil {
		panic(err)
	}
	
	fmt.Println("Parsed:", result)
}

```

## Examples

```
export GO111MODULE=auto
go build github.com/pkujhd/goloader/examples/loader

go tool compile $GOPATH/src/github.com/pkujhd/goloader/examples/schedule/schedule.go
./loader -o schedule.o -run main.main -times 10

go tool compile $GOPATH/src/github.com/pkujhd/goloader/examples/base/base.go
./loader -o base.o -run main.main

go tool compile $GOPATH/src/github.com/pkujhd/goloader/examples/http/http.go
./loader -o http.o -run main.main

go install github.com/pkujhd/goloader/examples/basecontext
go tool compile -I $GOPATH/pkg/`go env GOOS`_`go env GOARCH`/ $GOPATH/src/github.com/pkujhd/goloader/examples/inter/inter.go
./loader -o $GOPATH/pkg/`go env GOOS`_`go env GOARCH`/github.com/pkujhd/goloader/examples/basecontext.a:github.com/pkujhd/goloader/examples/basecontext -o inter.o

#build multiple go files
go tool compile -I $GOPATH/pkg/`go env GOOS`_`go env GOARCH`/ -o test.o test1.go test2.go
./loader -o test.o -run main.main

```

## Warning

Don't use "-s -w" compile argument, It strips symbol table.

This has currently only been tested and developed on:

Golang 1.8-1.19 (x64/x86, darwin, linux, windows)

Golang 1.10-1.19 (arm, linux, android)

Golang 1.8-1.19 (arm64, linux, android)

Golang 1.16-1.19 (arm64, darwin)
