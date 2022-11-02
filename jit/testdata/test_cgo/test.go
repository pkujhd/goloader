package test_cgo

import (
	"io"
	"net/http"
)

func MakeHTTPRequestWithDNS(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(resp.Body)
	return string(data), err
}
