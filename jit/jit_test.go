package jit_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/eh-steve/goloader"
	"github.com/eh-steve/goloader/jit"
	"github.com/eh-steve/goloader/jit/testdata/common"
	"github.com/eh-steve/goloader/jit/testdata/test_issue55/p"
	"github.com/eh-steve/goloader/jit/testdata/test_type_mismatch"
	"github.com/eh-steve/goloader/jit/testdata/test_type_mismatch/typedef"
	"github.com/eh-steve/goloader/unload/jsonunload"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

type testData struct {
	files []string
	pkg   string
}

// Can edit these flags to check all tests still work with different linker options
var baseConfig = jit.BuildConfig{
	GoBinary:                         "",
	KeepTempFiles:                    false,
	ExtraBuildFlags:                  nil,
	BuildEnv:                         os.Environ(),
	TmpDir:                           "",
	DebugLog:                         false,
	SymbolNameOrder:                  nil,
	RandomSymbolNameOrder:            false,
	RelocationDebugWriter:            nil,
	SkipTypeDeduplicationForPackages: nil,
	UnsafeBlindlyUseFirstmoduleTypes: false,
	Dynlink:                          os.Getenv("JIT_GC_DYNLINK") == "1",
}

func buildLoadable(t *testing.T, conf jit.BuildConfig, testName string, data testData) (module *goloader.CodeModule, symbols map[string]interface{}) {
	var loadable *jit.LoadableUnit
	var err error
	if os.Getenv("GOLOADER_DEBUG_RELOCATIONS") == "1" {
		conf.RelocationDebugWriter = os.Stderr
	}
	switch testName {
	case "BuildGoFiles":
		loadable, err = jit.BuildGoFiles(conf, data.files[0], data.files[1:]...)
	case "BuildGoPackage":
		loadable, err = jit.BuildGoPackage(conf, data.pkg)
	case "BuildGoText":
		var goText []byte
		goText, err = os.ReadFile(data.files[0])
		if err != nil {
			t.Fatal(err)
		}
		// cd into testdata dir to avoid polluting the jit package's go.mod, and cd back once finished
		pwd, err := os.Getwd()
		absFile, err := filepath.Abs(data.files[0])
		if err != nil {
			panic(err)
		}
		testdataDir := filepath.Dir(filepath.Dir(absFile))
		err = os.Chdir(testdataDir)
		if err != nil {
			panic(err)
		}
		defer func() {
			err = os.Chdir(pwd)
			if err != nil {
				panic(err)
			}
		}()
		loadable, err = jit.BuildGoText(conf, string(goText))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err != nil {
		t.Fatal(err)
	}

	if os.Getenv("GOLOADER_TEST_DUMP_SYMBOL_ORDER") == "1" {
		symOrder := loadable.Linker.SymbolOrder()
		symOrderJSON, _ := json.MarshalIndent(symOrder, "", "  ")
		f, err := os.CreateTemp("", "symbol_order_*.json")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write(symOrderJSON)
		_ = f.Close()
	}
	module, err = loadable.Load()
	if err != nil {
		t.Fatal(err)
	}
	symbols = module.SymbolsByPkg[loadable.ImportPath]
	return
}

func goVersion(t *testing.T) int64 {
	GoVersionParts := strings.Split(strings.TrimPrefix(runtime.Version(), "go"), ".")
	stripRC := strings.Split(GoVersionParts[1], "rc")[0] // Treat release candidates as if they are that version
	version, err := strconv.ParseInt(stripRC, 10, 64)
	if err != nil {
		t.Fatalf("failed to parse go version: %s", err)
	}
	return version
}

func TestJitSimpleFunctions(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_simple_func/test.go"},
		pkg:   "./testdata/test_simple_func",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			addFunc := symbols["Add"].(func(a, b int) int)
			result := addFunc(5, 6)
			if result != 11 {
				t.Errorf("expected %d, got %d", 11, result)
			}

			handleBytesFunc := symbols["HandleBytes"].(func(input interface{}) ([]byte, error))
			bytesOut, err := handleBytesFunc([]byte{1, 2, 3})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(bytesOut, []byte{1, 2, 3}) {
				t.Errorf("expected %v, got %v", []byte{1, 2, 3}, bytesOut)
			}
			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHeapStrings(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_simple_func/test.go"},
		pkg:   "./testdata/test_simple_func",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			testHeapStrings := symbols["TestHeapStrings"].(func() string)
			theString := testHeapStrings()

			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			runtime.GC()
			runtime.GC()
			fmt.Println(theString)
		})
	}
}

func TestJitJsonUnmarshal(t *testing.T) {
	conf := baseConfig
	data := testData{
		files: []string{"./testdata/test_json_unmarshal/test.go"},
		pkg:   "./testdata/test_json_unmarshal",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			MyFunc := symbols["MyFunc"].(func([]byte) (interface{}, error))
			result, err := MyFunc([]byte(`{"key": "value"}`))
			if err != nil {
				t.Fatal(err)
			}
			if result.(map[string]interface{})["key"] != "value" {
				t.Errorf("expected %s, got %v", "value", result)
			}
			jsonunload.Unload(module.DataAddr())
			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJitComplexFunctions(t *testing.T) {
	conf := baseConfig
	data := testData{
		files: []string{"./testdata/test_complex_func/test.go"},
		pkg:   "testdata/test_complex_func",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			complexFunc := symbols["ComplexFunc"].(func(input common.SomeStruct) (common.SomeStruct, error))
			result, err := complexFunc(common.SomeStruct{
				Val1:  []byte{1, 2, 3},
				Mutex: &sync.Mutex{},
			})
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(result.Val1.([]byte), []byte{3, 2, 1}) {
				t.Errorf("expected %d, got %d", []byte{3, 2, 1}, result.Val1)
			}

			newThingFunc := symbols["NewThing"].(func() common.SomeInterface)

			thing := newThingFunc()
			err = thing.Method2(map[string]interface{}{
				"item1": 5,
				"item2": 6,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err = thing.Method1(common.SomeStruct{
				Val1:  []byte{1, 2, 3},
				Mutex: &sync.Mutex{},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(result.Val1.([]byte), []byte{3, 2, 1}) {
				t.Errorf("expected %d, got %d", []byte{3, 2, 1}, result.Val1)
			}
			if result.Val2["item1"].(int) != 5 {
				t.Errorf("expected %d, got %d", 5, result.Val2["item1"])
			}
			if result.Val2["item2"].(int) != 6 {
				t.Errorf("expected %d, got %d", 6, result.Val2["item2"])
			}

			runtime.GC()
			runtime.GC()
			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJitEmbeddedStruct(t *testing.T) {
	conf := baseConfig
	data := testData{
		files: []string{"./testdata/test_embedded/test.go"},
		pkg:   "testdata/test_embedded",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			makeIt := symbols["MakeIt"].(func() int)
			result := makeIt()
			if result != 5 {
				t.Fatalf("expected 5, got %d", result)
			}

			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSchedule(t *testing.T) {
	conf := baseConfig
	data := testData{
		files: []string{"./testdata/test_schedule/test.go"},
		pkg:   "testdata/test_schedule",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			test := symbols["Test"].(func())
			for i := 0; i < 100; i++ {
				fmt.Println("Test ", i)
				test()
			}

			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJitCGoCall(t *testing.T) {
	if os.Getenv("CGO_ENABLED") == "0" {
		t.Skip("CGo disabled")
	}
	if runtime.GOOS == "windows" && goVersion(t) < 19 {
		t.Skip("PE relocs not yet supported on go 1.18")
	}
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_cgo/test.go"},
		pkg:   "testdata/test_cgo",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			cgoCall := symbols["CGoCall"].(func(a, b int32) (int32, int32, int32))
			mul, add, constant := cgoCall(2, 3)

			// This won't pass since nothing currently applies native elf/macho relocations in native code
			if mul != 6 {
				t.Errorf("expected mul to be 2 * 3 == 6, got %d", mul)
			}
			if add != 5 {
				t.Errorf("expected mul to be 2 + 3 == 5, got %d", add)
			}
			if constant != 5 {
				t.Errorf("expected constant to be 5, got %d", add)
			}
			fmt.Println(mul, add, constant)

			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJitCGoPackage(t *testing.T) {
	if os.Getenv("CGO_ENABLED") == "0" {
		t.Skip("CGo disabled")
	}
	if runtime.GOOS == "windows" {
		t.Skip("TODO - C calling Go not yet supported on Windows")
	}
	if os.Getenv("GITHUB_REPOSITORY") == "eh-steve/goloader" {
		t.Skip("I don't know why but this test fails in github CI")
	}
	conf := baseConfig

	data := testData{
		pkg: "testdata/test_cgo_package",
	}
	module, symbols := buildLoadable(t, conf, "BuildGoPackage", data)
	cgoCall := symbols["CGoCall"].(func(a, b int32) (int32, int32, int32, int32, int32))
	mul, add, constant, blah, cCallsGo := cgoCall(2, 3)

	// This won't pass since nothing currently applies native elf/macho relocations in native code
	if mul != 6 {
		t.Errorf("expected mul to be 2 * 3 == 6, got %d", mul)
	}
	if add != 5 {
		t.Errorf("expected mul to be 2 + 3 == 5, got %d", add)
	}
	if constant != 5 {
		t.Errorf("expected constant to be 5, got %d", add)
	}
	if blah != 999+5 {
		t.Errorf("expected blah to be 999+5, got %d", blah)
	}
	if cCallsGo != 30 {
		t.Errorf("expected cCallsGo to be 30, got %d", cCallsGo)
	}
	fmt.Println(mul, add, constant, blah, cCallsGo)

	err := module.Unload()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestJitHttpGet(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_http_get/test.go"},
		pkg:   "testdata/test_http_get",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			start := runtime.NumGoroutine()
			module, symbols := buildLoadable(t, conf, testName, data)
			httpGet := symbols["MakeHTTPRequestWithDNS"].(func(string) (string, error))
			result, err := httpGet("https://ipinfo.io/ip")
			if err != nil {
				t.Fatal(err)
			}
			afterCall := runtime.NumGoroutine()
			for afterCall > start {
				time.Sleep(100 * time.Millisecond)
				runtime.GC()
				afterCall = runtime.NumGoroutine()
				fmt.Printf("Waiting for last goroutine to stop before unloading, started with %d, now have %d\n", start, afterCall)
			}
			fmt.Println(len(result))
			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPatchMultipleModuleItabs(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_http_get/test.go"},
		pkg:   "testdata/test_http_get",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			start := runtime.NumGoroutine()
			module1, symbols1 := buildLoadable(t, conf, testName, data)
			module2, symbols2 := buildLoadable(t, conf, testName, data)
			httpGet1 := symbols1["MakeHTTPRequestWithDNS"].(func(string) (string, error))
			httpGet2 := symbols2["MakeHTTPRequestWithDNS"].(func(string) (string, error))
			result1, err := httpGet1("https://ipinfo.io/ip")
			if err != nil {
				t.Fatal(err)
			}
			// crypto/tls package caches server certificates in a sync.Map (tls.clientCertCache), and each JIT package would store a different
			// type of cache entry in the cache, causing a panic during type assertion, so GC twice to empty the cache before making the second request
			runtime.GC()
			runtime.GC()
			result2, err := httpGet2("https://ipinfo.io/ip")
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println(len(result2))
			afterCall := runtime.NumGoroutine()
			for afterCall != start {
				time.Sleep(100 * time.Millisecond)
				runtime.GC()
				afterCall = runtime.NumGoroutine()
				fmt.Printf("Waiting for last goroutine to stop before unloading, started with %d, now have %d\n", start, afterCall)
			}
			fmt.Println(len(result1))
			err = module1.Unload()
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(300 * time.Millisecond)

			result2, err = httpGet2("https://ipinfo.io/ip")
			fmt.Println(len(result2))
			if err != nil {
				t.Fatal(err)
			}
			err = module2.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPatchMultipleModuleItabsIssue55(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_issue55/t/t.go"},
		pkg:   "./testdata/test_issue55/t",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module1, symbols1 := buildLoadable(t, conf, testName, data)
			module2, symbols2 := buildLoadable(t, conf, testName, data)

			test1 := symbols1["Test"].(func(intf p.Intf) p.Intf)
			test2 := symbols2["Test"].(func(intf p.Intf) p.Intf)
			test1(&p.Stru{})
			test2(&p.Stru{})

			err := module1.Unload()
			if err != nil {
				t.Fatal(err)
			}

			test2(&p.Stru{})
			if err != nil {
				t.Fatal(err)
			}
			err = module2.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TODO - something wrong with this
func TestJitPanicRecoveryStackTrace(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_stack_trace/file1.go",
			"./testdata/test_stack_trace/file2.go",
			"./testdata/test_stack_trace/file3.go",
			"./testdata/test_stack_trace/file4.go",
			"./testdata/test_stack_trace/test.go"},
		pkg: "testdata/test_stack_trace",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			newThingFunc := symbols["NewThing"].(func() common.SomeInterface)

			thing := newThingFunc()
			err := checkStackTrace(t, thing)
			if err != nil {
				t.Fatal(err)
			}

			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func checkStackTrace(t *testing.T, thing common.SomeInterface) (err error) {
	defer func() {
		if v := recover(); v != nil {
			stack := debug.Stack()
			indices := make([]int, 9)
			orderedBytes := [][]byte{
				[]byte("/test.go:15"),
				[]byte("/file1.go:7"),
				[]byte(".(*SomeType).callSite1("),
				[]byte("/file2.go:11"),
				[]byte(".(*SomeType).callSite2("),
				[]byte("/file3.go:13"),
				[]byte(".(*SomeType).callSite3("),
				[]byte("/file4.go:16"),
				[]byte(".(*SomeType).callSite4("),
			}
			indices[0] = bytes.LastIndex(stack, orderedBytes[8])
			indices[1] = bytes.LastIndex(stack, orderedBytes[7])
			indices[2] = bytes.LastIndex(stack, orderedBytes[6])
			indices[3] = bytes.LastIndex(stack, orderedBytes[5])
			indices[4] = bytes.LastIndex(stack, orderedBytes[4])
			indices[5] = bytes.LastIndex(stack, orderedBytes[3])
			indices[6] = bytes.LastIndex(stack, orderedBytes[2])
			indices[7] = bytes.LastIndex(stack, orderedBytes[1])
			indices[8] = bytes.LastIndex(stack, orderedBytes[0])
			for i, index := range indices {
				if index == -1 {
					err = fmt.Errorf("expected stack trace to contain %s, but wasn't found, got \n%s", orderedBytes[8-i], stack)
					return
				}
			}
			if !sort.IsSorted(sort.IntSlice(indices)) {
				err = fmt.Errorf("expected stack trace to be ordered like %s, but got \n %s", orderedBytes, stack)
			}
		}
	}()
	_, err = thing.Method1(common.SomeStruct{Val1: "FECK"})
	if err != nil {
		t.Fatal(err)
	}
	return nil
}

func TestJitGoroutines(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_goroutines/test.go"},
		pkg:   "testdata/test_goroutines",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			newThing := symbols["NewThing"].(func() common.StartStoppable)
			thing := newThing()
			before := runtime.NumGoroutine()
			err := thing.Start()
			if err != nil {
				t.Fatal(err)
			}
			afterStart := runtime.NumGoroutine()
			thing.InChan() <- common.SomeStruct{Val1: "not working"}
			output := <-thing.OutChan()

			if output.Val1.(string) != "Goroutine working" {
				t.Fatalf("expected 'Goroutine working', got %s", output.Val1)
			}

			err = thing.Stop()
			time.Sleep(100 * time.Millisecond)
			afterStop := runtime.NumGoroutine()
			sleepCount := 0
			for afterStop != before {
				time.Sleep(100 * time.Millisecond)
				runtime.GC()
				afterStop = runtime.NumGoroutine()
				fmt.Printf("Waiting for last goroutine to stop before unloading, started with %d, now have %d\n", before, afterStop)
				sleepCount++
				if sleepCount > 20 {
					t.Fatalf("expected num goroutines %d and %d to be equal", before, afterStop)
				}
			}
			if afterStart != before+1 {
				t.Fatalf("expected afterStart to be 1 greater than before, got %d and %d", afterStart, before)
			}
			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoadUnloadMultipleModules(t *testing.T) {
	conf := baseConfig

	data1 := testData{
		files: []string{"./testdata/test_simple_func/test.go"},
		pkg:   "testdata/test_simple_func",
	}
	data2 := testData{
		files: []string{"./testdata/test_goroutines/test.go"},
		pkg:   "testdata/test_goroutines",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module1, symbols1 := buildLoadable(t, conf, testName, data1)
			module2, symbols2 := buildLoadable(t, conf, testName, data2)

			addFunc := symbols1["Add"].(func(a, b int) int)
			result := addFunc(5, 6)
			if result != 11 {
				t.Errorf("expected %d, got %d", 11, result)
			}

			handleBytesFunc := symbols1["HandleBytes"].(func(input interface{}) ([]byte, error))
			bytesOut, err := handleBytesFunc([]byte{1, 2, 3})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(bytesOut, []byte{1, 2, 3}) {
				t.Errorf("expected %v, got %v", []byte{1, 2, 3}, bytesOut)
			}

			newThing := symbols2["NewThing"].(func() common.StartStoppable)
			thing := newThing()
			before := runtime.NumGoroutine()
			err = thing.Start()
			if err != nil {
				t.Fatal(err)
			}
			afterStart := runtime.NumGoroutine()
			thing.InChan() <- common.SomeStruct{Val1: "not working"}
			output := <-thing.OutChan()

			if output.Val1.(string) != "Goroutine working" {
				t.Fatalf("expected 'Goroutine working', got %s", output.Val1)
			}

			err = thing.Stop()
			afterStop := runtime.NumGoroutine()
			sleepCount := 0
			for afterStop > before {
				time.Sleep(100 * time.Millisecond)
				runtime.GC()
				afterStop = runtime.NumGoroutine()
				fmt.Printf("Waiting for last goroutine to stop before unloading, started with %d, now have %d\n", before, afterStop)
				sleepCount++
				if sleepCount > 20 {
					t.Fatalf("expected num goroutines %d and %d to be equal", before, afterStop)
				}
			}
			if afterStart != before+1 {
				t.Fatalf("expected afterStart to be 1 greater than before, got %d and %d", afterStart, before)
			}

			// Don't unload in reverse order
			err = module1.Unload()
			if err != nil {
				t.Fatal(err)
			}
			err = module2.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStackMove(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_stack_move/test.go"},
		pkg:   "testdata/test_stack_move",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			RecurseUntilMaxDepth := symbols["RecurseUntilMaxDepth"].(func(depth int, oldAddr, prevDiff uintptr, splitCount int) int)

			var someVarOnStack int
			addr := uintptr(unsafe.Pointer(&someVarOnStack))

			stackMoveCount := RecurseUntilMaxDepth(0, addr, 144, 0)

			if stackMoveCount < 8 {
				// Depends on what beefy goroutine stacks are available to reuse when the test starts - if it gets a big one, there'll be fewer stack moves
				t.Errorf("expected at least 8 stack moves")
			}
			fmt.Println("Stack move count:", stackMoveCount)
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSimpleAsmFuncs(t *testing.T) {
	conf := baseConfig

	data := testData{
		pkg: "testdata/test_simple_asm_func",
	}
	testNames := []string{"BuildGoPackage"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			myMax := symbols["MyMax"].(func(a, b float64) float64)
			allMaxes := symbols["AllTheMaxes"].(func(a, b float64) (float64, float64, float64, float64))

			myMaxResult := myMax(5, 999)
			a, b, c, d := allMaxes(5, 999)
			if myMaxResult != 999 {
				t.Fatalf("expected myMaxResult to be 999, got %f", myMaxResult)
			}
			if a != 999 {
				t.Fatalf("expected a to be 999, got %f", a)
			}
			if b != 999 {
				t.Fatalf("expected b to be 999, got %f", b)
			}
			if c != 999 {
				t.Fatalf("expected c to be 999, got %f", c)
			}
			if d != 32 {
				t.Fatalf("expected d to be 32, got %f", d)
			}
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestComplexAsmFuncs(t *testing.T) {
	backupGoMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	backupGoSum, err := os.ReadFile("go.sum")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = os.WriteFile("go.mod", backupGoMod, os.ModePerm)
		if err != nil {
			t.Error(err)
		}
		err = os.WriteFile("go.sum", backupGoSum, os.ModePerm)
		if err != nil {
			t.Error(err)
		}
	}()

	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_complex_asm_func/test.go"},
		pkg:   "testdata/test_complex_asm_func",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			matPow := symbols["MatPow"].(func())

			matPow()
			matPow()
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// https://github.com/pkujhd/goloader/issues/55
func TestIssue55(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_issue55/t/t.go"},
		pkg:   "./testdata/test_issue55/t",
	}

	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			test := symbols["Test"].(func(intf p.Intf) p.Intf)
			test(&p.Stru{})
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// https://github.com/pkujhd/goloader/issues/78
func TestIssue78(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_issue78/test.go"},
		pkg:   "./testdata/test_issue78",
	}

	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			test := symbols["Test"].(func() (int, int))
			val1, val2 := test()
			if val1 != 2 {
				t.Fatalf("expected 2, got %d", val1)
			}
			if val2 != 2 {
				t.Fatalf("expected 2, got %d", val2)
			}
			val1, val2 = test()
			if val1 != 3 {
				t.Fatalf("expected 3, got %d", val1)
			}
			if val2 != 3 {
				t.Fatalf("expected 3, got %d", val2)
			}
			test2 := symbols["Test2"].(func() int)
			fmt.Printf("Reported: 0x%x\n", test2())
			test3 := symbols["Test3"].(func() int)
			val3 := test3()
			if val3 != common.Val {
				t.Fatalf("expected %d, got %d", common.Val, val3)
			}
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPackageNameNotEqualToImportPath(t *testing.T) {
	backupGoMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	backupGoSum, err := os.ReadFile("go.sum")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = os.WriteFile("go.mod", backupGoMod, os.ModePerm)
		if err != nil {
			t.Error(err)
		}
		err = os.WriteFile("go.sum", backupGoSum, os.ModePerm)
		if err != nil {
			t.Error(err)
		}
	}()

	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_package_path_not_import_path/test.go"},
		pkg:   "./testdata/test_package_path_not_import_path",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			whatever := symbols["Whatever"].(func())

			whatever()
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConvertOldAndNewTypes(t *testing.T) {
	var relocs io.WriteCloser
	if false {
		relocs, _ = os.OpenFile("relocs.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0655)
		defer relocs.Close()
	}
	conf := baseConfig
	conf.RelocationDebugWriter = relocs

	data := testData{
		files: []string{"./testdata/test_conversion/test.go"},
		pkg:   "testdata/test_conversion",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module1, symbols1 := buildLoadable(t, conf, testName, data)
			module2, symbols2 := buildLoadable(t, conf, testName, data)

			newThingFunc1 := symbols1["NewThingOriginal"].(func() common.SomeInterface)
			newThingFunc2 := symbols2["NewThingOriginal"].(func() common.SomeInterface)
			newThingIfaceFunc1 := symbols1["NewThingWithInterface"].(func() common.SomeInterface)
			newThingIfaceFunc2 := symbols2["NewThingWithInterface"].(func() common.SomeInterface)

			thing1 := newThingFunc1()
			thing2 := newThingFunc2()
			thingIface1 := newThingIfaceFunc1()
			thingIface2 := newThingIfaceFunc2()

			input := int64(123)
			out1, _ := thing1.Method1(common.SomeStruct{Val1: input, Val2: map[string]interface{}{}})
			current := out1.Val2["current"].(int64)
			if current != input {
				t.Fatalf("expected current to be the same as input: %d  %d", current, input)
			}

			newThing2, err := goloader.ConvertTypesAcrossModules(module1, module2, thing1, thing2)
			if err != nil {
				t.Fatal(err)
			}
			thing2 = newThing2.(common.SomeInterface)

			ifaceOut1, _ := thingIface1.Method1(common.SomeStruct{Val1: input, Val2: map[string]interface{}{}})
			ifaceCounter1 := ifaceOut1.Val2["exclusive_interface_counter"].(string)
			byteReader1 := ifaceOut1.Val2["bytes_reader_output"].([]byte)
			ifaceCurrent1 := ifaceOut1.Val2["current"].(int64)
			ifaceCurrentComplex1 := ifaceOut1.Val2["complex"].(map[interface{}]interface{})

			ifaceOut12, _ := thingIface1.Method1(common.SomeStruct{Val1: []byte{4, 5, 6}, Val2: map[string]interface{}{}})
			ifaceCounter12 := ifaceOut12.Val2["exclusive_interface_counter"].(string)
			byteReader12 := ifaceOut12.Val2["bytes_reader_output"].([]byte)
			ifaceCurrent12 := ifaceOut12.Val2["current"].(int64)
			ifaceCurrentComplex12 := ifaceOut12.Val2["complex"].(map[interface{}]interface{})
			_ = thingIface1.Method2(nil)

			newThingIface2, err := goloader.ConvertTypesAcrossModules(module1, module2, thingIface1, thingIface2)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println(thingIface1)
			fmt.Println(newThingIface2)

			thingIface2 = newThingIface2.(common.SomeInterface)

			// Unload thing1's types + methods entirely
			err = module1.Unload()

			if err != nil {
				t.Fatal(err)
			}

			ifaceOut2, _ := thingIface2.Method1(common.SomeStruct{Val1: 789, Val2: map[string]interface{}{}})

			ifaceCounter2 := ifaceOut2.Val2["exclusive_interface_counter"].(string)
			byteReader2 := ifaceOut2.Val2["bytes_reader_output"].([]byte)
			ifaceCurrent2 := ifaceOut2.Val2["current"].(int64)

			if ifaceCounter1 != "Counter: 124" {
				t.Fatalf("expected ifaceCounter1 to be 'Counter: 124', got %s", ifaceCounter1)
			}
			if ifaceCounter12 != "Counter: 125" {
				t.Fatalf("expected ifaceCounter12 to be 'Counter: 125', got %s", ifaceCounter12)
			}
			if ifaceCounter2 != "Counter: 126" {
				t.Fatalf("expected ifaceCounter2 to be 'Counter: 126', got %s", ifaceCounter2)
			}
			if !bytes.Equal(byteReader1, []byte{1, 2, 3}) {
				t.Fatalf("expected byteReader1 to be []byte{1,2,3}, got %v", byteReader1)
			}
			if !bytes.Equal(byteReader12, []byte{4, 5, 6}) {
				t.Fatalf("expected byteReader12 to be []byte{4,5,6}, got %v", byteReader12)
			}
			if !bytes.Equal(byteReader12, []byte{4, 5, 6}) {
				t.Fatalf("expected byteReader2 to be []byte{4,5,6}, got %v", byteReader2)
			}
			if !bytes.Equal(byteReader12, []byte{4, 5, 6}) {
				t.Fatalf("expected byteReader2 to be []byte{4,5,6}, got %v", byteReader2)
			}
			if !reflect.DeepEqual(ifaceCurrentComplex1, ifaceCurrentComplex12) {
				t.Fatalf("expected ifaceCurrentComplex12 to be %v, got %v", ifaceCurrentComplex1, ifaceCurrentComplex12)
			}
			out2, _ := thing2.Method1(common.SomeStruct{Val1: nil, Val2: map[string]interface{}{}})
			converted2 := out2.Val2["current"].(int64)
			if converted2 != input {
				t.Fatalf("expected converted to be the same as input: %d  %d", converted2, input)
			}
			_ = thingIface2.Method2(nil)

			fmt.Println(ifaceCurrent1, ifaceCurrent12, ifaceCurrent2)
			if err != nil {
				t.Fatal(err)
			}

			err = module2.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// This test works and demonstrates the preservation of state across modules, but to avoid affecting what
// parts of net/http gets baked into the test binary for the earlier TestJitHttpGet, it is commented out.

/**

type SwapperMiddleware struct {
	handler http.Handler
	mutex   sync.RWMutex
}

func (h *SwapperMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	h.handler.ServeHTTP(w, r)
}

func TestStatefulHttpServer(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_stateful_server/test.go"},
		pkg:   "./testdata/test_stateful_server",
	}

	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			makeHandler := symbols["MakeServer"].(func() http.Handler)
			handler := makeHandler()
			server := &http.Server{
				Addr: "localhost:9091",
			}
			h := &SwapperMiddleware{handler: handler}

			server.Handler = h
			go func() {
				_ = server.ListenAndServe()
			}()
			time.Sleep(time.Millisecond * 100)
			resp, err := http.Post("http://localhost:9091", "text/plain", strings.NewReader("test1"))
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			fmt.Println(string(body))

			module2, symbols2 := buildLoadable(t, conf, testName, data)

			makeHandler2 := symbols2["MakeServer"].(func() http.Handler)
			handler2 := makeHandler2()

			newHandler, err := goloader.ConvertTypesAcrossModules(module, module2, handler, handler2)
			if err != nil {
				t.Fatal(err)
			}

			h.mutex.Lock()
			h.handler = newHandler.(http.Handler)
			h.mutex.Unlock()

			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}

			resp, err = http.Post("http://localhost:9091", "text/plain", strings.NewReader("test2"))
			if err != nil {
				t.Fatal(err)
			}
			body, _ = io.ReadAll(resp.Body)
			fmt.Println(string(body))

			err = module2.Unload()
			if err != nil {
				t.Fatal(err)
			}

			server.Close()
		})
	}
}

*/

func TestCloneConnection(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_clone_connection/test.go"},
		pkg:   "testdata/test_clone_connection",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	listener, err := net.Listen("tcp", ":9091")
	if err != nil {
		t.Fatal(err)
	}
	keepAccepting := true
	var results [][]string
	go func() {
		connectionCount := 0
		for keepAccepting {
			conn, err := listener.Accept()
			connectionCount++
			var result []string
			results = append(results, result)
			if err != nil {
				if keepAccepting {
					t.Error("expected to continue accepting", err)
				}
				return
			}
			go func(c net.Conn, index int) {
				buf := make([]byte, 8)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					results[index-1] = append(results[index-1], string(buf[:n]))
				}
			}(conn, connectionCount)
		}
	}()

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module1, symbols1 := buildLoadable(t, conf, testName, data)
			module2, symbols2 := buildLoadable(t, conf, testName, data)

			newDialerFunc1 := symbols1["NewConnDialer"].(func() common.MessageWriter)
			newDialerFunc2 := symbols2["NewConnDialer"].(func() common.MessageWriter)

			dialer1 := newDialerFunc1()
			dialer2 := newDialerFunc2()

			err = dialer1.Dial("localhost:9091")

			if err != nil {
				t.Fatal(err)
			}
			_, err = dialer1.WriteMessage("test1234")
			if err != nil {
				t.Fatal(err)
			}

			newDialer2, err := goloader.ConvertTypesAcrossModules(module1, module2, dialer1, dialer2)
			if err != nil {
				t.Fatal(err)
			}
			err = module1.Unload()
			if err != nil {
				t.Fatal(err)
			}
			dialer2 = newDialer2.(common.MessageWriter)
			_, err = dialer2.WriteMessage("test5678")
			if err != nil {
				t.Fatal(err)
			}
			err = dialer2.Close()

			err = module2.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if len(results) != len(testNames) {
		t.Errorf("expected %d connection test results, got %d", len(testNames), len(results))
	}
	for _, result := range results {
		if len(result) != 2 {
			t.Errorf("expected 2 writes per connection, got %d", len(result))
		} else {
			if result[0] != "test1234" {
				t.Errorf("expected first write to be test1234, got %s", result[0])
			}
			if result[1] != "test5678" {
				t.Errorf("expected second write to be test5678, got %s", result[1])
			}
		}
	}
	keepAccepting = false
	_ = listener.Close()
}

func TestJitSBSSMap(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_init/test.go"},
		pkg:   "./testdata/test_init",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			printMap := symbols["PrintMap"].(func() string)
			if printMap() != "map[blah:map[5:6 7:8] blah_blah:map[1:2 3:4]]" {
				t.Errorf("expected map string to be 'map[blah:map[5:6 7:8] blah_blah:map[1:2 3:4]]', got %s\n", printMap())
			}
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJitDefer(t *testing.T) {
	conf := baseConfig
	// If the symbol "gonum.org/v1/gonum/mat.(*LU).updateCond.opendefer" is added before others pertaining to (*LU).updateCond, this test will fail with the fatal error:
	// runtime: g 73: unexpected return pc for gonum.org/v1/gonum/mat.(*LU).updateCond.func1 called from 0xc004283700
	// TODO - investigate why
	conf.RandomSymbolNameOrder = false

	data := testData{
		files: []string{"./testdata/test_defer_funcs/test.go"},
		pkg:   "./testdata/test_defer_funcs",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				module, symbols := buildLoadable(t, conf, testName, data)
				testDefer := symbols["TestOpenDefer"].(func())
				testDefer()
				runtime.GC()
				err := module.Unload()
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestAnonymousStructType(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_anonymous_struct_type/test.go"},
		pkg:   "./testdata/test_anonymous_struct_type",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			testFunc := symbols["Test"].(func() bool)
			fmt.Println(testFunc())
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProtobuf(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_protobuf/test.go"},
		pkg:   "./testdata/test_protobuf",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			testFunc := symbols["TestProto"].(func())
			testFunc()
			runtime.GC()
			runtime.GC()
			err := module.Unload()
			runtime.GC()
			runtime.GC()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// This test is commented to avoid adding protobuf as a dependency to the jit package purely for a test
// TODO - split out all tests into a separate package/module so they can add dependencies more freely

//func TestProtobufUnload(t *testing.T) {
//	conf := baseConfig
//	_ = os.Setenv("GOLANG_PROTOBUF_REGISTRATION_CONFLICT", "warn")
//
//	data := testData{
//		files: []string{"./testdata/test_protobuf/test.go"},
//		pkg:   "./testdata/test_protobuf",
//	}
//	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
//	for _, testName := range testNames {
//		t.Run(testName, func(t *testing.T) {
//			module1, symbols1 := buildLoadable(t, conf, testName, data)
//			module2, symbols2 := buildLoadable(t, conf, testName, data)
//			testFunc1 := symbols1["TestProto"].(func())
//			testFunc2 := symbols2["TestProto"].(func())
//			testFunc1()
//			runtime.GC()
//			runtime.GC()
//			protobufunload.Unload(module1.DataAddr())
//			err := module1.Unload()
//			runtime.GC()
//			runtime.GC()
//			if err != nil {
//				t.Fatal(err)
//			}
//			testFunc2()
//			runtime.GC()
//			runtime.GC()
//			protobufunload.Unload(module2.DataAddr())
//			err = module2.Unload()
//			runtime.GC()
//			runtime.GC()
//			if err != nil {
//				t.Fatal(err)
//			}
//		})
//	}
//}

func TestK8s(t *testing.T) {
	if goVersion(t) < 19 {
		t.Skip("k8s requires 1.19+")
	}
	if runtime.GOOS == "windows" {
		t.Skip("k8s requires golang/x/sys/windows to init which causes error: Failed to load kernel32.dll: The system cannot find the path specified (possibly a problem in golang.org/x/sys?)")
	}

	conf := baseConfig
	conf.UnsafeBlindlyUseFirstmoduleTypes = true
	data := testData{
		files: []string{"./testdata/test_k8s/test.go"},
		pkg:   "./testdata/test_k8s",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			testFunc := symbols["TryK8s"].(func())
			testFunc()

			runtime.GC()
			runtime.GC()
			runtime.GC()
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGCGlobals(t *testing.T) {
	conf := baseConfig

	if runtime.GOOS == "windows" && goVersion(t) < 19 {
		t.Skip("this test is broken on go1.18 windows")
	}
	data := testData{
		files: []string{"./testdata/test_gc_globals/test.go"},
		pkg:   "./testdata/test_gc_globals",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			testFunc := symbols["Find"].(func(lat, lon float64) string)
			for i := 0; i < 100; i++ {
				testFunc(55, 55)
				runtime.GC()
				runtime.GC()
			}

			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPprofIssue75(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_pprof/test.go"},
		pkg:   "./testdata/test_pprof",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			server := http.Server{
				Addr: ":6060",
			}
			defer server.Close()
			go func() {
				log.Println(server.ListenAndServe())
			}()

			keepRequesting := atomic.Value{}
			keepRequesting.Store(true)
			requestCount := 0
			go func() {
				r, err := http.NewRequest("GET", "http://localhost:6060/debug/pprof/heap?debug=1", nil)
				if err != nil {
					t.Fatal(err)
				}
				for keepRequesting.Load() == true {
					respRec := httptest.NewRecorder()
					http.DefaultServeMux.ServeHTTP(respRec, r)
					if respRec.Code != 200 {
						t.Errorf("Got a non-200 response: %d %s", respRec.Code, respRec.Body.String())
					}
					requestCount++
				}
			}()
			testFunc := symbols["TestPprofIssue75"].(func() int)
			for i := 0; i < 1000; i++ {
				testFunc()
			}
			keepRequesting.Store(false)
			fmt.Println(requestCount)
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond * 100)
		})
	}
}

func TestJson(t *testing.T) {
	conf := baseConfig

	data := testData{
		files: []string{"./testdata/test_json_marshal/test.go"},
		pkg:   "./testdata/test_json_marshal",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}
	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			testFunc := symbols["TestJSONMarshal"].(func() string)

			if testFunc() != "1" {
				t.Fatalf("expected \"1\" but got %s", testFunc())
			}
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTypeMismatch(t *testing.T) {
	conf := baseConfig
	conf.UnsafeBlindlyUseFirstmoduleTypes = false // If set to true, this test should fail (fault)

	data := testData{
		files: []string{"./testdata/test_type_mismatch/test.go"},
		pkg:   "./testdata/test_type_mismatch",
	}
	// If the test shows a type deduplication bug, there will be a hard fault due to out of heap memory access
	// We still want to run the deferred file writes to restore the testdata, so we SetPanicOnFault
	debug.SetPanicOnFault(true)

	originalFileType, err := os.ReadFile("./testdata/test_type_mismatch/typedef/typedef.go")
	if err != nil {
		t.Fatal(err)
	}
	newFileType, err := os.ReadFile("./testdata/test_type_mismatch/typedef/different_type.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.WriteFile("./testdata/test_type_mismatch/typedef/typedef.go", originalFileType, 0655)
	}()

	module, symbols := buildLoadable(t, conf, "BuildGoPackage", data)
	bakedIn := test_type_mismatch.New()
	testFunc := symbols["New"].(func() *typedef.Thing)
	dynamic := testFunc()

	if reflect.TypeOf(bakedIn) != reflect.TypeOf(dynamic) {
		t.Errorf("expected types to be the same %p, %p", reflect.TypeOf(bakedIn), reflect.TypeOf(dynamic))
	}

	err = module.Unload()
	if err != nil {
		t.Fatal(err)
	}

	// Replace the file with a new, incompatible type
	err = os.WriteFile("./testdata/test_type_mismatch/typedef/typedef.go", newFileType, 0655)
	if err != nil {
		t.Fatal(err)
	}

	module2, symbols2 := buildLoadable(t, conf, "BuildGoPackage", data)

	testFunc2, ok := symbols2["New"].(func() *typedef.Thing)

	if ok {
		t.Error("expected function type to be different")
		dynamic2 := testFunc2() // This will probably hard fault due to the incorrect type being used in a relocation

		if reflect.TypeOf(bakedIn) == reflect.TypeOf(dynamic2) {
			t.Errorf("expected types to be different %p, %p", reflect.TypeOf(bakedIn), reflect.TypeOf(dynamic2))
		}
	} else {
		testFunc3 := reflect.ValueOf(symbols2["New"])
		result := testFunc3.Call(nil)

		if reflect.TypeOf(bakedIn) == result[0].Type() {
			t.Errorf("expected types to be different %p, %p", reflect.TypeOf(bakedIn), result[0].Type())
		}
	}

	err = module2.Unload()
	if err != nil {
		t.Fatal(err)
	}

}

func TestRemotePkgs(t *testing.T) {
	// This test tries to build some massive real world packages as a smoke test.
	// Ideally in future we'd also build and run the tests for those packages as JIT modules to prove everything works
	conf := baseConfig
	conf.UnsafeBlindlyUseFirstmoduleTypes = true // Want to speed this up so avoid building stuff we know hasn't changed

	remotePackagesToBuild := []string{
		"gonum.org/v1/gonum/mat", // gonum has plenty of asm
	}

	if goVersion(t) >= 19 && runtime.GOOS != "windows" {
		remotePackagesToBuild = append(remotePackagesToBuild,
			"k8s.io/client-go/kubernetes", // K8s is a whopper
			"k8s.io/client-go/rest")       // also hefty
	}
	for _, pkg := range remotePackagesToBuild {
		loadable, err := jit.BuildGoPackageRemote(conf, pkg, "latest")
		if err != nil {
			t.Fatal(err)
		}
		module, err := loadable.Load()
		if err != nil {
			t.Fatal(err)
		}
		syms := module.SymbolsByPkg[loadable.ImportPath]
		for k, v := range syms {
			fmt.Println(loadable.ImportPath, k, reflect.TypeOf(v))
		}
		// To clean up sync.Pools before unload
		runtime.GC()
		runtime.GC()
		err = module.Unload()
		if err != nil {
			t.Fatal(err)
		}
	}

	// And now something specific
	loadable, err := jit.BuildGoPackageRemote(conf, "encoding/json", "latest")
	if err != nil {
		t.Fatal(err)
	}
	module, err := loadable.Load()
	if err != nil {
		t.Fatal(err)
	}
	syms := module.SymbolsByPkg[loadable.ImportPath]

	jsonMarshalIndent := syms["MarshalIndent"].(func(interface{}, string, string) ([]byte, error))
	data, err := jsonMarshalIndent(conf, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(data))
	if err != nil {
		t.Fatal(err)
	}
	err = module.Unload()
	if err != nil {
		t.Fatal(err)
	}
}
