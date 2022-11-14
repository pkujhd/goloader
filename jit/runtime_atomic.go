package jit

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"unsafe"
)

//go:linkname Casuintptr runtime/internal/atomic.Casuintptr
func Casuintptr(ptr *uintptr, old, new uintptr) bool

//go:linkname Load runtime/internal/atomic.Load
func Load(ptr *uint32) uint32

//go:linkname Loadp runtime/internal/atomic.Loadp
func Loadp(ptr unsafe.Pointer) unsafe.Pointer

//go:linkname Loaduintptr runtime/internal/atomic.Loaduintptr
func Loaduintptr(ptr *uintptr) uintptr

//go:linkname Store runtime/internal/atomic.Store
func Store(ptr *uint32, val uint32)

//go:linkname Storeuintptr runtime/internal/atomic.Storeuintptr
func Storeuintptr(ptr *uintptr, new uintptr)

//go:linkname Xadd runtime/internal/atomic.Xadd
func Xadd(ptr *uint32, delta int32) uint32

//go:linkname Xadduintptr runtime/internal/atomic.Xadduintptr
func Xadduintptr(ptr *uintptr, delta uintptr) uintptr

//go:linkname Xchg runtime/internal/atomic.Xchg
func Xchg(ptr *uint32, new uint32) uint32

//go:linkname Xchguintptr runtime/internal/atomic.Xchguintptr
func Xchguintptr(ptr *uintptr, new uintptr) uintptr

//go:linkname Cas runtime/internal/atomic.Cas
func Cas(ptr *uint32, old, new uint32) bool

//go:linkname Cas64 runtime/internal/atomic.Cas64
func Cas64(ptr *uint64, old, new uint64) bool

//go:linkname Load64 runtime/internal/atomic.Load64
func Load64(ptr *uint64) uint64

//go:linkname Store64 runtime/internal/atomic.Store64
func Store64(ptr *uint64, val uint64)

//go:linkname Xadd64 runtime/internal/atomic.Xadd64
func Xadd64(ptr *uint64, delta int64) uint64

//go:linkname Xchg64 runtime/internal/atomic.Xchg64
func Xchg64(ptr *uint64, new uint64) uint64

//go:linkname Or8 runtime/internal/atomic.Or8
func Or8(ptr *uint8, val uint8)

//go:linkname And8 runtime/internal/atomic.And8
func And8(ptr *uint8, val uint8)

//go:linkname Compare internal/bytealg.Compare
func Compare(a, b []byte) int

//go:linkname Index internal/bytealg.Index
func Index(a, b []byte) int

//go:linkname IndexString internal/bytealg.IndexString
func IndexString(a, b string) int

//go:linkname IndexByte internal/bytealg.IndexByte
func IndexByte(a []byte, b byte) int

//go:linkname IndexByteString internal/bytealg.IndexByteString
func IndexByteString(s string, b byte) int

//go:linkname Count internal/bytealg.Count
func Count(b []byte, c byte) int

//go:linkname CountString internal/bytealg.CountString
func CountString(b string, c byte) int

var test_z64, test_x64 uint64

//go:linkname complex128div runtime.complex128div
func complex128div(n complex128, m complex128) complex128

func testAtomic64() {
	test_z64 = 42
	test_x64 = 0
	Cas64(&test_z64, test_x64, 1)
	Cas64(&test_z64, test_x64, 1)
	Load64(&test_z64)
	Store64(&test_z64, (1<<40)+1)
	Xadd64(&test_z64, (1<<40)+1)
	Xchg64(&test_z64, (3<<40)+3)
}

func check() {
	var (
		a uintptr
		m [4]byte
		z uint32
	)
	z = 1
	Cas(&z, 1, 2)
	Load(&z)
	Loadp(unsafe.Pointer(&a))
	Loaduintptr(&a)
	Store(&z, 1)
	Storeuintptr(&a, 1)
	Xadduintptr(&a, 0)
	Xadd(&z, 0)
	Xchg(&z, 0)
	Xchguintptr(&a, 0)
	Casuintptr(&a, 5, 6)
	Or8(&m[1], 0xf0)
	And8(&m[1], 0x1)
	testAtomic64()
	Compare(nil, nil)
	Count(nil, 0)
	CountString("", 0)
	Index([]byte{0}, []byte{0})
	IndexString("\x00", "0")
	IndexByte([]byte{0}, 0)
	IndexByteString("\x00", 0)

	complex128div(2+2i, 3+3i)

	// To bake in the deeper edges of reflect
	_, _, _ = reflect.Select([]reflect.SelectCase{{
		Dir:  reflect.SelectSend,
		Chan: reflect.MakeChan(reflect.TypeOf(make(chan int)), 1),
		Send: reflect.ValueOf(1),
	}})

	// To bake in internal/reflectlite
	var i interface{} = 5
	errors.As(nil, &i)

	// To bake in all of sync.Cond's methods referencing functions defined in runtime since runtime is a forbidden package
	var _ = reflect.ValueOf(sync.Cond{})
	var _ = reflect.DeepEqual(1, 2)
	var _ = reflect.MakeFunc(reflect.TypeOf(func() {}), nil)
	_ = bytes.Compare(nil, nil)

	// encoding/json's encoderCache sync.Map can cache functions for encoding builtin types, but these functions
	// might not be baked in, and so could be unloaded. To avoid caching dynamically loaded code, we force inclusion of
	// functions for encoding all the builtin types
	type forcedJson struct {
		I     []*int
		U     []*uint
		F32   []*float32
		F64   []*float64
		S     []*string
		Iface []*interface{}
		Stru  []*struct {
			I int
		}
		Msi map[string]int
	}
	f := forcedJson{}
	_, _ = json.Marshal(&f)
}
