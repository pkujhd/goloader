package test_json_marshal

import (
	"encoding/json"
)

type impl struct {
	a int
}

func (impl) MarshalJSON() ([]byte, error) {
	return []byte("1"), nil
}

func TestJSONMarshal() string {
	i := impl{}
	b, _ := json.Marshal(&i)
	return string(b)
}
