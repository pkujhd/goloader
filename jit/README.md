# JIT Compiler for Go

This package attempts to streamline the runtime build/load process for arbitrary Go code.

It assumes the existence of a working Go toolchain on the path.

It automatically resolves package dependencies recursively, and provides a type safe way of interacting with the built
functions.

### Usage and Configuration

```go
package main

import (
	"fmt"
	"github.com/pkujhd/goloader/jit"
)

func main() {
	conf := jit.BuildConfig{
		KeepTempFiles:    false,          // Files are copied/written to a temp dir to ensure it is writable. This retains the temporary copies
		ExtraBuildFlags:  []string{"-x"}, // Flags passed to go build command
		BuildEnv:         nil,            // Env vars to set for go build toolchain
		TmpDir:           "",             // To control where temporary files are copied
		DebugLog:         true,           //
		SkipCopyPatterns: []string{},     // Glob patterns to exclude from module copy (in case module contains large files)
	}

	loadable, err := jit.BuildGoFiles(conf, "./path/to/file1.go", "/path/to/file2.go")
	if err != nil {
		panic(err)
	}
	// or
	loadable, err = jit.BuildGoPackage(conf, "./path/to/package")
	if err != nil {
		panic(err)
	}
	// or
	loadable, err = jit.BuildGoText(conf, `
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

	module, symbols, err := loadable.Load()

	if err != nil {
		panic(err)
	}
	defer func() {
		err = module.Unload()
		if err != nil {
			panic(err)
		}
	}()
	switch f := symbols["MyFunc"].(type) {
	case func([]byte) (interface{}, error):
		result, err := f([]byte(`{"k":"v"}`))
		if err != nil {
			panic(err)
		}
		fmt.Println(result)
	default:
		panic("Function signature was not what was expected")
	}
}

```