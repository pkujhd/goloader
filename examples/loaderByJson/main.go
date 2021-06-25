package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/pkujhd/goloader"
)

type ObjInfo struct {
	PackageName string
	ObjFilePath string
	ObjFileHash string
}

type Builder struct {
	ObjInfos    []ObjInfo
	ExecuteFunc []string
	packageName string
	packagePath string
}

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
	var jsonFilePath = flag.String("j", "./goloader.json", "json file path")
	flag.Parse()
	file, err := os.Open(*jsonFilePath)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Println(err)
		return
	}
	var files arrayFlags
	b := &Builder{}
	json.Unmarshal(data, b)
	for _, info := range b.ObjInfos {
		// FIXME: Optimize this conditional judgment.
		if info.PackageName != "fmt" && info.PackageName != "runtime" {
			files.Set(info.ObjFilePath + ":" + info.PackageName)
		}
	}

	if len(files.File) == 0 {
		flag.PrintDefaults()
		return
	}

	symPtr := make(map[string]uintptr)
	err = goloader.RegSymbol(symPtr)
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

	linker, err := goloader.ReadObjs(files.File, files.PkgPath)
	if err != nil {
		fmt.Println(err)
		return
	}

	var mmapByte []byte
	codeModule, err := goloader.Load(linker, symPtr)
	if err != nil {
		fmt.Println("Load error:", err)
		return
	}

	for _, fn := range b.ExecuteFunc {
		runFuncPtr := codeModule.Syms[fn]
		if runFuncPtr == 0 {
			fmt.Println("Load error! not find function:", fn)
			return
		}
		funcPtrContainer := (uintptr)(unsafe.Pointer(&runFuncPtr))
		runFunc := *(*func())(unsafe.Pointer(&funcPtrContainer))
		runFunc()
	}

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
