package httptest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
)

type serverMock struct {
	Method   string
	Path     string
	Response any
	Request  map[string]any
	Header   http.Header
	Srv      *httptest.Server
}

// NewServerMock creates a test server that responds with the given response when called with the given method and path.
// Make sure to close the server after the test is done.
// Server will try to decode the request body into a map[string]any.
func NewServerMock(method string, path string, response any) *serverMock {
	req := make(map[string]any)
	header := make(map[string][]string)
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if r.Method != method {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		for k, vv := range map[string][]string(r.Header) {
			header[k] = vv
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if response != nil {
			if err := json.NewEncoder(w).Encode(response); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}))

	fmt.Println("header:", header)
	return &serverMock{
		Method:   method,
		Path:     path,
		Response: response,
		Header:   header,
		Request:  req,
		Srv:      ts,
	}
}
