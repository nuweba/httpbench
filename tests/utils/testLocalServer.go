package tests

import (
	"net/http"
	"net/http/httptest"
)

func RunLocalServer(f func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(f))
	return ts
}
