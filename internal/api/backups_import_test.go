package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestAnImportIsStagedNotAppliedAndCanBeCancelled(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.EnsureLocalOwner(); err != nil {
		t.Fatal(err)
	}
	const key = "0123456789abcdef0123456789abcdef"
	sealer, _ := backup.NewSealer(key, "test")
	svc := backup.New(st, backup.Config{Dir: filepath.Join(dir, "backups"), Sealer: sealer, Usable: true, KeySource: "test", Key: key})
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := svc.Archive(context.Background(), 0, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, meta.Fetcher{}, "", WithAuthMode(AuthNone), WithBackups(svc)))
	defer ts.Close()

	do := func(method string, body []byte) (int, map[string]any) {
		req, _ := http.NewRequest(method, ts.URL+"/api/v2/admin/backups/import", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return resp.StatusCode, v
	}
	if code, v := do("GET", nil); code != 200 || v["pending"] != nil {
		t.Fatalf("nothing should be pending: %d %v", code, v)
	}
	code, v := do("POST", raw)
	if code != 200 || v["staged"] != true || v["restartRequired"] != true {
		t.Fatalf("stage: %d %v", code, v)
	}
	// A running controller keeps serving the database it has; the copy waits
	// for the next start.
	if _, err := os.Stat(filepath.Join(dir, "pending-import", "reet.db")); err != nil {
		t.Fatalf("not staged on disk: %v", err)
	}
	if code, v := do("GET", nil); code != 200 || v["pending"] == nil {
		t.Fatalf("pending: %d %v", code, v)
	}
	if code, _ := do("POST", []byte("not a sealed copy")); code != 400 {
		t.Fatalf("garbage should be refused, got %d", code)
	}
	if code, _ := do("DELETE", nil); code != 200 {
		t.Fatalf("cancel: %d", code)
	}
	if p, _ := backup.PendingImport(dir); p != nil {
		t.Fatal("still pending after cancel")
	}
}
