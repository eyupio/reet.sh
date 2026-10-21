package main

import (
	"context"
	"github.com/eyupio/reet.sh/internal/store"
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

func TestServerRegistersOwnerEnvironment(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	owner, err := st.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("REET_CONTROLLER_URL", "")
	t.Setenv("REET_ENV", "test-server")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reportServer(ctx, st)
	envs, err := st.SyncEnvironmentsFor(owner)
	if err != nil || len(envs) != 1 || envs[0].Agent != "reet-server" || envs[0].Host != "test-server" || envs[0].OS == "" {
		t.Fatalf("missing server identity: %+v %v", envs, err)
	}
}
