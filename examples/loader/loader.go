package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/pkujhd/goloader"
)

type arrayFlags struct {
	File    []string
	PkgPath []string
}

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	s := strings.Split(value, ":")
	i.File = append(i.File, s[0])
	var path string
	if len(s) > 1 {
		path = s[1]
	}
	i.PkgPath = append(i.PkgPath, path)
	return nil
}

func main() {
	var files arrayFlags
	flag.Var(&files, "o", "load go object file")
	var pkgpath = flag.String("p", "", "package path")
	var parseFile = flag.String("parse", "", "parse go object file")
	var run = flag.String("run", "main.main", "run function")
	var times = flag.Int("times", 1, "run count")

	flag.Parse()

	if *parseFile != "" {
		parse(*parseFile, *pkgpath)
		return
	}

	if len(files.File) == 0 {
		flag.PrintDefaults()
		return
	}

	symPtr := make(map[string]uintptr)
	err := goloader.RegSymbol(symPtr)
	if err != nil {
		fmt.Println(err)
		return
	}

	// most of time you don't need to register function, but if loader complain about it, you have to.
	w := sync.WaitGroup{}
	str := make([]string, 0)
	goloader.RegTypes(symPtr, http.ListenAndServe, http.Dir("/"),
		http.Handler(http.FileServer(http.Dir("/"))), http.FileServer, http.HandleFunc,
		&http.Request{}, &http.Server{}, (&http.ServeMux{}).Handle)
	goloader.RegTypes(symPtr, runtime.LockOSThread, &w, w.Wait)
	goloader.RegTypes(symPtr, fmt.Sprint, str)

	linker, err := goloader.ReadObjs(files.File, files.PkgPath)
	if err != nil {
		fmt.Println(err)
		return
	}

	var mmapByte []byte
	for i := 0; i < *times; i++ {
		codeModule, err := goloader.Load(linker, symPtr)
		if err != nil {
			fmt.Println("Load error:", err)
			return
		}
		runFuncPtr := codeModule.Syms[*run]
		if runFuncPtr == 0 {
			fmt.Println("Load error! not find function:", *run)
			return
		}
		funcPtrContainer := (uintptr)(unsafe.Pointer(&runFuncPtr))
		runFunc := *(*func())(unsafe.Pointer(&funcPtrContainer))
		runFunc()
		os.Stdout.Sync()
		codeModule.Unload()

		// a strict test, try to make mmap random
		if mmapByte == nil {
			mmapByte, err = goloader.Mmap(1024)
			if err != nil {
				fmt.Println(err)
			}
			b := make([]byte, 1024)
			copy(mmapByte, b) // reset all bytes
		} else {
			goloader.Munmap(mmapByte)
			mmapByte = nil
		}
	}

}

func parse(file, pkgpath string) {
	if file == "" {
		flag.PrintDefaults()
		return
	}
	obj, err := goloader.Parse(file, pkgpath)
	fmt.Printf("%# v\n", obj)
	if err != nil {
		fmt.Printf("error reading %s: %v\n", file, err)
		return
	}
}
