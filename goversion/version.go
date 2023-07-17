package goversion

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

func GoVersion() int64 {
	GoVersionParts := strings.Split(strings.TrimPrefix(runtime.Version(), "go"), ".")
	stripRC := strings.Split(GoVersionParts[1], "rc")[0] // Treat release candidates as if they are that version
	version, err := strconv.ParseInt(stripRC, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse go version: %s", err))
	}
	return version
}
