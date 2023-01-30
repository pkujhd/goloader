package test_json_unmarshal

import "encoding/json"

func MyFunc(input []byte) (interface{}, error) {
	var output interface{}
	err := json.Unmarshal(input, &output)
	return output, err
}
