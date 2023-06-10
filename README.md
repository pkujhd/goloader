# Goloader/JIT Compiler for Go

[![Build Status](https://github.com/eh-steve/goloader/actions/workflows/go.yml/badge.svg)](https://github.com/eh-steve/goloader/actions/workflows/go.yml)

The `goloader/jit` package can compile and load Go code from text, file, folder or remote package (including code with
package imports).

It automatically resolves package dependencies recursively, and provides a type safe way of interacting with the built
functions.

Forked from [dearplain](https://github.com/dearplain/goloader) and [pkujhd](https://github.com/pkujhd/goloader).

# Usage

## Build

**Make sure you're using go >= 1.18.**

First, execute the following command. This is because Goloader relies on the internal package, which is forbidden by the
Go compiler.

```
cp -r $GOROOT/src/cmd/internal $GOROOT/src/cmd/objfile
```

## Go compiler patch

To allow the loader to know the types of exported functions, this package will attempt to patch the Go compiler (gc) to
emit these if not already patched.

The effect of the patch can be found in [`jit/gc.patch`](https://github.com/eh-steve/goloader/blob/master/jit/gc.patch).

```bash
go install github.com/eh-steve/goloader/jit/patchgc@latest
# You may need to run patchgc as sudo if your $GOROOT is owned by root
# (alternatively `chown -R $USER:$USER $GOROOT`)
patchgc
```

## Example Usage

```go
package main

import (
	"fmt"
	"github.com/eh-steve/goloader/jit"
)

func main() {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,          // Files are copied/written to a temp dir to ensure it is writable. This retains the temporary copies
		ExtraBuildFlags: []string{"-x"}, // Flags passed to go build command
		BuildEnv:        nil,            // Env vars to set for go build toolchain
		TmpDir:          "",             // To control where temporary files are copied
		DebugLog:        true,           //
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
	loadable, err = jit.BuildGoPackageRemote(conf, "github.com/some/package/v4", "latest")
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

	module, err := loadable.Load()
	// module.SymbolsByPkg is a map[string]map[string]interface{} of all packages and their exported functions and global vars
	symbols := module.SymbolsByPkg[loadable.ImportPath]
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

## How does it work?

Goloader works like a linker, it relocates the addresses of symbols in an object file, generates runnable code, and then
reuses the runtime functions and the type pointers of the loader where available.

Goloader provides some information to the runtime and garbage collector of Go, which allows it to work correctly with
them.

Please note that Goloader is not a scripting engine. It reads the archives emitted from the Go compiler and makes them
runnable. All features of Go are supported, and run just as fast and lightweight as native Go code.

## Comparison with `plugin`

Plugin:

* Can't load plugins not built with exact same versions of packages that host binary
  uses (`plugin was built with a different version of package`) - this makes them basically unusable in most large
  projects
* Introduces dependency on `libdl`/CGo (and doesn't work on Windows)
* Prevents linker deadcode elimination for unreachable methods (increases host binary size with unused methods)
* Can't be unloaded/dynamically updated
* Duplicates a lot of the go runtime (large binary sizes)

Goloader:

* Can build/load any packages (somewhat unsafely - it attempts to verify that types across JIT packages and host
  packages match, but doesn't do the same checks for function signatures)
* Pure Go - no dependency on `libdl`/Cgo
* Patches host itabs containing unreachable methods instead of preventing linker deadcode elimination
* Can be unloaded, and objects from one version of a JIT package can be converted at runtime to those from another
  version, to allow dynamic adjustment of functions/methods without losing state
* Reuses the runtime from the host binary (much smaller binaries)

Goloader supports pprof tool (yes, you can see code loaded by Goloader in pprof), but does not (yet) support debugging
with `delve`.

## OS/Arch Compatibility

JIT compiler tested/passing on:

| **OS/Arch**        | amd64/+CGo         | arm64/+CGo         | amd64/-CGo         | arm64/-CGo         |
|--------------------|--------------------|--------------------|--------------------|--------------------|
| Linux/go-1.20.3    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Darwin/go-1.20.3   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Windows/go-1.20.3  | :heavy_check_mark: | :interrobang:      | :heavy_check_mark: | :interrobang:      |
| Linux/go-1.19.4    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Darwin/go-1.19.4   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Windows/go-1.19.4  | :heavy_check_mark: | :interrobang:      | :heavy_check_mark: | :interrobang:      |
| Linux/go-1.18.8    | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Darwin/go-1.18.8   | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Windows/go-1.18.8  | :x:                | :interrobang:      | :heavy_check_mark: | :interrobang:      |

## Warning

Don't use "-s -w" compile argument, It strips symbol table.
