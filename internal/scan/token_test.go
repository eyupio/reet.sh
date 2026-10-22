package scan

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanEnvFileSendsTokenWithoutLogging(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REET_TOKEN", "")
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("REET_TOKEN=fixture-secret\n"), 0600)
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("X-Reet-Token") != "fixture-secret" {
			t.Error("missing token")
		}
		w.Write([]byte(`{"imported":0,"errors":[]}`))
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := Run([]string{"--server", srv.URL, "--env-file", p}, &out); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || strings.Contains(out.String(), "fixture-secret") {
		t.Fatal("request/output contract failed")
	}
	if err := Run([]string{"--dry-run", "--env-file", "missing"}, &out); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatal("dry run contacted server")
	}
}
