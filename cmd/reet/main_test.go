package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheckOKAndFailure(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer ok.Close()
	if err := healthcheck(ok.URL + "/healthz"); err != nil {
		t.Fatalf("healthy server: %v", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) }))
	defer bad.Close()
	if err := healthcheck(bad.URL + "/healthz"); err == nil {
		t.Fatal("500 should fail")
	}
	if err := healthcheck("http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("refused connection should fail")
	}
}

func TestHealthURLFromAddr(t *testing.T) {
	for in, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080/healthz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/healthz",
		"127.0.0.1:7000": "http://127.0.0.1:7000/healthz",
	} {
		if got := healthURL(in); got != want {
			t.Errorf("healthURL(%q) = %q, want %q", in, got, want)
		}
	}
}
