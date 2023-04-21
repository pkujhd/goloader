package main

import (
	"github.com/eh-steve/goloader/jit"
	"log"
	"os"
)

func main() {
	var goBinaryPath = "go"
	if len(os.Args) > 1 {
		goBinaryPath = os.Args[1]
	}
	err := jit.PatchGC(goBinaryPath, true)
	if err != nil {
		log.Fatalln(err)
	}
}
