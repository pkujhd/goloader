package test_anonymous_struct_type

import (
	"fmt"
	"sync"
)

// try to reproduce a problem from net/hosts.go and net/dnsclient_unix.go on arm64

var hosts struct {
	sync.Mutex
	byName map[string][]string
}

func Test() bool {
	hosts.Lock()
	defer hosts.Unlock()

	type result struct {
		m      sync.Mutex
		server string
		error
	}

	lane := make(chan result, 1)
	fmt.Println(cap(lane))

	if len(hosts.byName) == 0 {
		return true
	}

	return false
}
