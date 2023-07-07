package test_protobuf

import (
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func TestProto() {
	fmt.Println(timestamppb.New(time.Now()).String())
}
