package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckOKAndFailure(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer ok.Close()
	result, err := Check(context.Background(), ok.URL+"/healthz")
	if err != nil || !result.OK || result.Status != http.StatusOK {
		t.Fatalf("healthy server: %+v %v", result, err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) }))
	defer bad.Close()
	result, err = Check(context.Background(), bad.URL+"/healthz")
	if err == nil || result.OK || result.Status != 500 {
		t.Fatalf("unhealthy server: %+v %v", result, err)
	}
}

func TestEndpointAndURLFromAddr(t *testing.T) {
	for target, want := range map[string]string{
		"https://reet.example":          "https://reet.example/healthz",
		"https://reet.example/base":     "https://reet.example/base/healthz",
		"http://127.0.0.1:8080/healthz": "http://127.0.0.1:8080/healthz",
	} {
		got, err := Endpoint(target)
		if err != nil || got != want {
			t.Errorf("Endpoint(%q) = %q, %v; want %q", target, got, err, want)
		}
	}
	if _, err := Endpoint("https://user:secret@example.test"); err == nil {
		t.Fatal("accepted URL credentials")
	}
	if got := URLFromAddr(":8080"); got != "http://127.0.0.1:8080/healthz" {
		t.Fatalf("URLFromAddr = %q", got)
	}
}
