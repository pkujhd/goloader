package test_goroutines

import "github.com/eh-steve/goloader/jit/testdata/common"

type Thing struct {
	inChan   chan common.SomeStruct
	outChan  chan common.SomeStruct
	stopChan chan struct{}
}

func NewThing() common.StartStoppable {
	return &Thing{
		inChan:   make(chan common.SomeStruct),
		outChan:  make(chan common.SomeStruct),
		stopChan: make(chan struct{}),
	}
}
func (t *Thing) Start() error {
	go func() {
		for {
			select {
			case input := <-t.inChan:
				input.Val1 = "Goroutine working"
				t.outChan <- input
			case <-t.stopChan:
				return
			}
		}
	}()
	return nil
}

func (t *Thing) Stop() error {
	t.stopChan <- struct{}{}
	return nil
}

func (t *Thing) InChan() chan<- common.SomeStruct {
	return t.inChan
}

func (t *Thing) OutChan() <-chan common.SomeStruct {
	return t.outChan
}
