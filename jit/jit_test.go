package jit_test

import (
	"bytes"
	"fmt"
	"github.com/pkujhd/goloader"
	"github.com/pkujhd/goloader/jit"
	"github.com/pkujhd/goloader/jit/testdata/common"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"testing"
	"unsafe"
)

type testData struct {
	files []string
	pkg   string
}

func buildLoadable(t *testing.T, conf jit.BuildConfig, testName string, data testData) (module *goloader.CodeModule, symbols map[string]interface{}) {
	var loadable *jit.LoadableUnit
	var err error
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
		loadable, err = jit.BuildGoText(conf, string(goText))
	}
	if err != nil {
		t.Fatal(err)
	}
	module, symbols, err = loadable.Load()
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestJitSimpleFunctions(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        false,
	}

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

func TestJitComplexFunctions(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        true,
	}

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

			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJitEmbeddedStruct(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        false,
	}

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
		})
	}
}

// TODO - something wrong with this
func TestJitCGoCall(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        false,
	}

	data := testData{
		files: []string{"./testdata/test_cgo/test.go"},
		pkg:   "testdata/test_cgo",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			httpGet := symbols["MakeHTTPRequestWithDNS"].(func(string) (string, error))
			result, err := httpGet("https://ipinfo.io/ip")
			if err != nil {
				t.Fatal(err)
			}

			fmt.Println(result)
			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TODO - something wrong with this
func TestJitPanicRecoveryStackTrace(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        false,
	}

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
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        false,
	}

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
			afterStop := runtime.NumGoroutine()
			if before != afterStop {
				t.Fatalf("expected num goroutines %d and %d to be equal", before, afterStop)
			}
			if afterStart != before+1 {
				t.Fatalf("expected afterStart to be 1 greater than before, got %d and %d", afterStart, before)
			}
			err = module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoadUnloadMultipleModules(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        false,
	}

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
			if before != afterStop {
				t.Fatalf("expected num goroutines %d and %d to be equal", before, afterStop)
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

func TestStackSplit(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        false,
	}

	data := testData{
		files: []string{"./testdata/test_stack_split/test.go"},
		pkg:   "testdata/test_stack_split",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage", "BuildGoText"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)
			RecurseUntilMaxDepth := symbols["RecurseUntilMaxDepth"].(func(depth int, oldAddr, prevDiff uintptr, splitCount int) int)

			var someVarOnStack int
			addr := uintptr(unsafe.Pointer(&someVarOnStack))

			splitCount := RecurseUntilMaxDepth(0, addr, 144, 0)

			if splitCount < 12 {
				t.Errorf("expected at least 12 stack splits")
			}
			fmt.Println("Split count:", splitCount)
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSimpleAsmFuncs(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        false,
	}

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
		})
	}
}

func TestComplexAsmFuncs(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "../tmp",
		DebugLog:        false,
	}

	data := testData{
		files: []string{"./testdata/test_complex_asm_func/test.go"},
		pkg:   "testdata/test_complex_asm_func",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage"}

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
		})
	}
}

func TestPackageNameNotEqualToImportPath(t *testing.T) {
	conf := jit.BuildConfig{
		KeepTempFiles:   false,
		ExtraBuildFlags: nil,
		BuildEnv:        nil,
		TmpDir:          "",
		DebugLog:        true,
	}

	data := testData{
		files: []string{"./testdata/test_package_path_not_import_path/test.go"},
		pkg:   "./testdata/test_package_path_not_import_path",
	}
	testNames := []string{"BuildGoFiles", "BuildGoPackage"}

	for _, testName := range testNames {
		t.Run(testName, func(t *testing.T) {
			module, symbols := buildLoadable(t, conf, testName, data)

			whatever := symbols["Whatever"].(func())

			whatever()
			err := module.Unload()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
