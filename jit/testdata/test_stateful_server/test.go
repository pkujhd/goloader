package test_stateful_server

import (
	"encoding/json"
	"io"
	"net/http"
)

func MakeServer() http.Handler {
	return &StatefulHandler{
		remoteAddrs: map[customInternalStorage]struct{}{},
	}
}

type customInternalStorage struct {
	RemoteAddrs string
}

type StatefulHandler struct {
	LastRequestData []string
	RequestCount    int
	remoteAddrs     map[customInternalStorage]struct{}
	RemoteAddrs     []string
}

func (s *StatefulHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	data, _ := io.ReadAll(request.Body)
	s.LastRequestData = append(s.LastRequestData, string(data))
	s.remoteAddrs[customInternalStorage{request.RemoteAddr}] = struct{}{}
	s.RemoteAddrs = append(s.RemoteAddrs, request.RemoteAddr)
	s.RequestCount++
	result, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
	_, _ = writer.Write(result)
}
