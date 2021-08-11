package main

import (
	"fmt"
	"net/http"
)

type SimpleHanle struct{}

func (*SimpleHanle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello goloader!"))
}

func main() {
	go func() {
		panic(http.ListenAndServe(":2300", http.FileServer(http.Dir("."))))
	}()
	var inter http.Handler
	inter = &SimpleHanle{}
	listen(inter)
}

func listen(inter interface{}) {
	mux := http.NewServeMux()
	mux.Handle("/", inter.(http.Handler))
	fmt.Println("start listen:9090")
	panic(http.ListenAndServe(":9090", mux))
}
