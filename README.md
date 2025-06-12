
# Goloader

![Build Status](https://github.com/pkujhd/goloader/workflows/goloader%20Testing/badge.svg)

Goloader can load and run Golang code at runtime.

Forked from **https://github.com/dearplain/goloader**, Take over maintenance because the original author is not in maintenance

## How does it work?

Goloader works like a linker: it relocates the address of symbols in an object file, generates runnable code, and then reuses the runtime function and the type pointer of the loader.

Goloader provides some information to the runtime and gc of Go, which allows it to work correctly with them.

Please note that Goloader is not a scripting engine. It reads the output of Go compiler and makes them runnable. All features of Go are supported, and run just as fast and lightweight as native Go code.

## Comparison with plugin

Goloader reuses the Go runtime, which makes it much smaller. And code loaded by Goloader is unloadable.

Goloader supports pprof tool(Yes, you can see code loaded by Goloader in pprof). 

## Build

**Make sure you're using go >= 1.8.**

First, execute the following command, then do build and test. This is because Goloader relies on the internal package, which is forbidden by the Go compiler.
```
  cp -r $GOROOT/src/cmd/internal $GOROOT/src/cmd/objfile
```

## Examples

#### Build Loader:

If use go version >= 1.23
```
  go build --ldflags="-checklinkname=0" github.com/pkujhd/goloader/examples/loader
```
If use go version <= 1.22
```
  go build github.com/pkujhd/goloader/examples/loader
```


#### Compile Object File and Run:

If use go module or go version >= 1.20
```
  export GO111MODULE=on
  go list -export -deps -f '{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}' $GOPATH/src/github.com/pkujhd/goloader/examples/schedule/schedule.go > importcfg
  go tool compile -importcfg importcfg $GOPATH/src/github.com/pkujhd/goloader/examples/schedule/schedule.go
  ./loader -o schedule.o -run main.main -times 10
```
If use go path and go version < 1.20
```
  export GO111MODULE=auto
  go tool compile $GOPATH/src/github.com/pkujhd/goloader/examples/schedule/schedule.go
  ./loader -o schedule.o -run main.main -times 10
  
  go tool compile $GOPATH/src/github.com/pkujhd/goloader/examples/base/base.go
  ./loader -o base.o -run main.main
  
  go tool compile $GOPATH/src/github.com/pkujhd/goloader/examples/http/http.go
  ./loader -o http.o -run main.main
  
  go install github.com/pkujhd/goloader/examples/basecontext
  go tool compile -I $GOPATH/pkg/`go env GOOS`_`go env GOARCH`/ $GOPATH/src/github.com/pkujhd/goloader/examples/inter/inter.go
  ./loader -o $GOPATH/pkg/`go env GOOS`_`go env GOARCH`/github.com/pkujhd/goloader/examples/basecontext.a:github.com/pkujhd/goloader/examples/basecontext -o inter.o
```


#### Build multiple go files
```
  go tool compile -I $GOPATH/pkg/`go env GOOS`_`go env GOARCH`/ -o test.o test1.go test2.go
  ./loader -o test.o -run main.main
```

## compile with goloaderbuilder

#### compile only package archive
```
./builder -f $GOPATH/src/github.com/pkujhd/goloader/examples/inter/inter.go -p inter -b
./loader -o ./target/github.com/pkujhd/goloader/examples/inter/inter.a -run inter.main -times 10
```

#### compile with dependence packages
```
./builder -e ../runner/runner -f $GOPATH/src/github.com/pkujhd/goloader/examples/inter/inter.go -p inter
../runner/runner -f target/inter.goloader -r inter.main
```


## Warning

Don't use "-s -w" compile argument, It strips symbol table.

Don't use "go run" and "go test" command, "-s -w" compile argument is default.

This has currently only been tested and developed on:

Golang 1.8-1.24 (x64/x86, darwin, linux, windows)

Golang 1.10-1.24 (arm, linux, android)

Golang 1.8-1.24 (arm64, linux, android)

Golang 1.16-1.24 (arm64, darwin)
