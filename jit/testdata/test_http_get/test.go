package test_http_get

import (
	"io"
	"net/http"
	"runtime"
	"time"
)

func MakeHTTPRequestWithDNS(url string) (string, error) {
	// Make the IdleConnTimeout very short so that we don't leave dangling goroutines
	// reading those connections while the module wants to unload that code
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.IdleConnTimeout = time.Millisecond * 50
	http.DefaultClient.Transport = t
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	// Sleep for longer than the IdleConnTimeout so that all connections try to close themselves
	time.Sleep(100 * time.Millisecond)

	// GC to clean up all sync.Pools used by crypto/tls and http etc. (otherwise the pools will be attempted in the
	// next GC cycle which would be too late, and the memory would already be munmapped)
	defer runtime.GC()
	defer resp.Body.Close() // Release the body back to the http client reader pool

	// Don't leave any goroutines attempting to read idle connections since the code they execute will be unloaded
	defer http.DefaultClient.CloseIdleConnections()

	data, err := io.ReadAll(resp.Body)
	return string(data), err
}
