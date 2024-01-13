package inter

import (
	"fmt"

	"github.com/pkujhd/goloader/examples/basecontext"
)

func init() {
	fmt.Println("inter package init")
}

func scontextPrint(i basecontext.ISContext) {
	fmt.Println("IN FUNC scontextPrint start")
	fmt.Println("scontextPrint", i.GetName())
	fmt.Println("scontextPrint", i.GetInfo())
	i.PrintInfo()
	i.PrintName()
	fmt.Println("IN FUNC scontextPrint end")
}

func bcontextPrint(i basecontext.IBaseContext) {
	i.PrintName()
	fmt.Println("bcontextPrint", i.GetName())
}
func main() {
	var scontext basecontext.TSContext
	var bcontext basecontext.TBaseContext
	scontext.SetInfo("Context Info")
	scontext.SetName("Context Name")
	bcontext.SetName("BaseContext Name")
	scontextPrint(&scontext)
	bcontextPrint(&bcontext)
}
