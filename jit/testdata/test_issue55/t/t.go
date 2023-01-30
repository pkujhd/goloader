package t

import (
	"fmt"
	"github.com/pkujhd/goloader/jit/testdata/test_issue55/p"
)

func Test(param p.Intf) p.Intf {
	param.Print("Intf")
	fmt.Println("done!")
	return param
}
