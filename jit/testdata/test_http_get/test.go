package test_http_get

import (
	"io"
	"net/http"
	"runtime"
)

func MakeHTTPRequestWithDNS(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	// Clean up all sync.Pools used by crypto/tls and http etc
	defer runtime.GC()
	defer resp.Body.Close()
	// Don't leave any goroutines attempting to read idle connections since the code they execute will be unloaded
	defer http.DefaultClient.CloseIdleConnections()
	data, err := io.ReadAll(resp.Body)
	return string(data), err
}
