package test_conversion

import (
	"bytes"
	"fmt"
	"github.com/pkujhd/goloader/jit/testdata/common"
	"io"
	"os"
)

type ConvertibleOriginal struct {
	someInt int64
	myMap   map[string]interface{}
}

type ExclusiveInterface interface {
	ExclusivelyHere() string
	AddCount(int)
}

type scalarAlias float64

type CyclicInterface interface {
	Cyclic()
}

type cyclicDataStructureInner struct {
	self CyclicInterface
}

type cyclicDataStructure struct {
	a int64
	cyclicDataStructureInner
	b int
}

func (c *cyclicDataStructure) Cyclic() {

}

func newCyclicDataStructure() *cyclicDataStructure {
	c := cyclicDataStructure{}
	c.self = &c
	return &c
}

type ConvertibleWithInterface struct {
	someString                        string
	someInt                           int64
	theInterfaceNil                   io.ReadSeeker
	theInterface                      io.ReadSeeker
	theEmptyInterfaceWithExternalType interface{}
	theEmptyInterfaceWithCustomType   interface{}
	chanOfExternalType                chan int
	chanOfCustomType                  chan *exclHereImpl
	arrayOfExternalType               [77]int
	arrayOfCustomType                 [77]*exclHereImpl
	arrayOfCustomInterface            [77]ExclusiveInterface
	mapKeyedByExternalType            map[string]interface{}
	mapKeyedByCustomType              map[*exclHereImpl]interface{}
	mapValuesExternalType             map[interface{}]string
	mapValuesCustomType               map[string]*exclHereImpl
	sliceOfExternalType               []int64
	sliceOfCustomType                 []*exclHereImpl
	sliceOfExternalInterface          []interface{}
	sliceOfCustomInterface            []ExclusiveInterface
	structVal                         exclHereImpl
	structPtrVal                      *exclHereImpl
	scalarAlias                       scalarAlias
	anotherInterfaceOnlyHere          ExclusiveInterface
	cyclicDataStructure               *cyclicDataStructure
	funcValExternal                   func(string) ([]byte, error)
	funcValCustom                     func(int) *exclHereImpl
	funcValMethod                     func(int) *exclHereImpl
	funcValItabMethod                 func(int) *exclHereImpl
	funcValClosure                    func(int) *exclHereImpl
}

func NewThingOriginal() common.SomeInterface {
	return &ConvertibleOriginal{
		myMap: map[string]interface{}{},
	}
}

func NewThingWithInterface() common.SomeInterface {
	return &ConvertibleWithInterface{
		someInt:                           int64(5),
		theInterface:                      bytes.NewReader([]byte{1, 2, 3}),
		theEmptyInterfaceWithExternalType: int64(5),
		theEmptyInterfaceWithCustomType:   &exclHereImpl{},
		anotherInterfaceOnlyHere:          &exclHereImpl{counter: 0},
		mapValuesExternalType:             map[interface{}]string{},
		mapValuesCustomType:               map[string]*exclHereImpl{},
		mapKeyedByCustomType:              map[*exclHereImpl]interface{}{},
		mapKeyedByExternalType:            map[string]interface{}{},
	}
}

func (c *ConvertibleOriginal) Method1(input common.SomeStruct) (common.SomeStruct, error) {
	if intVal, ok := input.Val1.(int64); ok {
		c.someInt = intVal
	}
	input.Val2["current"] = c.someInt
	return input, nil
}

func (c *ConvertibleOriginal) Method2(input map[string]interface{}) error {
	return nil
}

func customFunc(i int) *exclHereImpl {
	return &exclHereImpl{counter: i}
}

type thing struct {
	i int
}

type someInterface interface {
	customFunc(i int) *exclHereImpl
}

func (t *thing) customFunc(i int) *exclHereImpl {
	t.i += i
	var x ExclusiveInterface = &exclHereImpl{counter: i}
	t.otherFunc(x)
	return x.(*exclHereImpl)
}

func (t *thing) otherFunc(x ExclusiveInterface) {
	x.AddCount(t.i)
	fmt.Printf("receiver func %d %p\n", t.i, t.customFunc)
}

func (c *ConvertibleWithInterface) Method1(input common.SomeStruct) (common.SomeStruct, error) {
	if intVal, ok := input.Val1.(int64); ok {
		c.someString = "test"
		c.someInt = intVal
		c.anotherInterfaceOnlyHere.AddCount(int(intVal))
		c.structPtrVal = c.anotherInterfaceOnlyHere.(*exclHereImpl)
		c.structVal = *c.structPtrVal
		c.funcValExternal = os.ReadFile
		c.funcValCustom = customFunc
		thing := &thing{6}
		var thingAsIface someInterface = thing
		c.funcValMethod = thing.customFunc
		c.funcValItabMethod = thingAsIface.customFunc
		c.funcValClosure = func(i int) *exclHereImpl {
			fmt.Println("Inside closure")
			return thing.customFunc(i)
		}
		c.mapKeyedByExternalType["test"] = 5
		c.mapKeyedByCustomType[c.structPtrVal] = bytes.NewReader(nil)
		c.mapValuesCustomType["test"] = c.structPtrVal
		c.mapValuesExternalType[c.structPtrVal] = "test"
		c.sliceOfCustomType = append(c.sliceOfCustomType, c.structPtrVal)
		c.sliceOfExternalInterface = append(c.sliceOfExternalInterface, "test")
		c.sliceOfCustomInterface = append(c.sliceOfCustomInterface, c.structPtrVal)
		c.sliceOfExternalType = append(c.sliceOfExternalType, intVal)
		c.scalarAlias = scalarAlias(intVal)
		c.chanOfExternalType = make(chan int, 10)
		c.chanOfCustomType = make(chan *exclHereImpl, 10)
		c.cyclicDataStructure = newCyclicDataStructure()
	}
	if bytesVal, ok := input.Val1.([]byte); ok {
		c.theInterface = bytes.NewReader(bytesVal)
		c.theInterfaceNil = bytes.NewReader(bytesVal)
		for i, v := range bytesVal {
			c.arrayOfExternalType[i] = int(v)
			c.arrayOfCustomType[i] = c.structPtrVal
			c.arrayOfCustomInterface[i] = c.structPtrVal
		}
	}
	bytesContent, err := io.ReadAll(c.theInterface)

	if err != nil {
		panic(err)
	}
	_, err = c.theInterface.Seek(0, 0)
	if err != nil {
		panic(err)
	}
	input.Val2["current"] = c.someInt
	input.Val2["exclusive_interface_counter"] = c.anotherInterfaceOnlyHere.ExclusivelyHere()
	input.Val2["bytes_reader_output"] = bytesContent
	return input, nil
}

func (c *ConvertibleWithInterface) Method2(input map[string]interface{}) error {
	c.funcValCustom(c.structVal.counter)
	fmt.Printf("method 2 called custom %p\n", c.funcValCustom)
	c.funcValMethod(c.structVal.counter)
	fmt.Printf("method 2 called method %p\n", c.funcValMethod)
	c.funcValItabMethod(c.structVal.counter)
	fmt.Printf("method 2 called itab method %p\n", c.funcValItabMethod)
	c.funcValClosure(c.structVal.counter)
	fmt.Printf("method 2 called closure %p\n", c.funcValItabMethod)
	return nil
}

type exclHereImpl struct {
	counter int
}

func (e *exclHereImpl) ExclusivelyHere() string {
	e.counter++
	return fmt.Sprintf("Counter: %d", e.counter)
}

func (e *exclHereImpl) AddCount(i int) {
	e.counter += i
}
