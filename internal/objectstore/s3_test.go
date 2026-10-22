package objectstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestClientRoundTrip(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unsigned", http.StatusForbidden)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/bucket/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			objects[key] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		case http.MethodDelete:
			delete(objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client, err := New(Config{Endpoint: server.URL, Bucket: "bucket", AccessKey: "key", SecretKey: "secret", PathStyle: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.Put(ctx, "prefix/object", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	got, err := client.Get(ctx, "prefix/object")
	if err != nil || !bytes.Equal(got, []byte("payload")) {
		t.Fatalf("get: %q %v", got, err)
	}
	if err := client.Delete(ctx, "prefix/object"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(ctx, "prefix/object"); err == nil {
		t.Fatal("deleted object was readable")
	}
}

func TestNewRejectsIncompleteConfig(t *testing.T) {
	for _, config := range []Config{
		{},
		{Endpoint: "ftp://example.test", Bucket: "bucket", AccessKey: "key", SecretKey: "secret"},
		{Endpoint: "https://example.test", AccessKey: "key", SecretKey: "secret"},
		{Endpoint: "https://example.test", Bucket: "bucket", SecretKey: "secret"},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("incomplete config accepted: %+v", config)
		}
	}
}

func boolPtr(value bool) *bool { return &value }
