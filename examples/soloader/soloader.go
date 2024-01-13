package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"github.com/pkujhd/goloader"
)

import "C"

//export loader
func loader(name, run, selfpath string) {
	symPtr := make(map[string]uintptr)
	err := goloader.RegSymbolWithPath(symPtr, selfpath)
	if err != nil {
		fmt.Println(err)
		return
	}

	// most of time you don't need to register function, but if loader complain about it, you have to.
	w := sync.WaitGroup{}
	goloader.RegTypes(symPtr, http.ListenAndServe, http.Dir("/"),
		http.Handler(http.FileServer(http.Dir("/"))), http.FileServer, http.HandleFunc,
		&http.Request{}, &http.Server{})
	goloader.RegTypes(symPtr, runtime.LockOSThread, &w, w.Wait)
	goloader.RegTypes(symPtr, fmt.Sprint)

	var linker *goloader.Linker
	linker, err = goloader.ReadObj(name, "")
	if err != nil {
		fmt.Println(err)
		return
	}

	var codeModule *goloader.CodeModule
	codeModule, err = goloader.Load(linker, symPtr)
	if err != nil {
		fmt.Println("Load error:", err)
		return
	}
	runFuncPtr := codeModule.Syms[run]
	if runFuncPtr == 0 {
		fmt.Println("Load error! not find function:", run)
		fmt.Println(codeModule.Syms)
		return
	}
	funcPtrContainer := (uintptr)(unsafe.Pointer(&runFuncPtr))
	runFunc := *(*func())(unsafe.Pointer(&funcPtrContainer))
	runFunc()
	os.Stdout.Sync()
	codeModule.Unload()
}

func main() {

}
