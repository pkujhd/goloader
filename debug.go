package goloader

import (
	"fmt"

	"github.com/pkujhd/goloader/objabi/dataindex"
)

func dumpStackMap(f interface{}) {
	finfo := findfunc(getFunctionPtr(f))
	fmt.Println(funcname(finfo))
	stkmap := (*stackmap)(funcdata(finfo, dataindex.FUNCDATA_LocalsPointerMaps))
	fmt.Printf("%v %p\n", stkmap, stkmap)
}
