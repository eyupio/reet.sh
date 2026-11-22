package api

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func gzipTestGet(t *testing.T, h http.Handler, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", "/x", nil)
	if acceptEncoding != "" {
		r.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	compressJSON(h).ServeHTTP(rec, r)
	return rec
}

func TestCompressJSONLargeBodyRoundTrips(t *testing.T) {
	doc := map[string]string{"text": strings.Repeat("the export reads each ledger line ", 200)}
	want, _ := json.Marshal(doc)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, doc)
	})
	rec := gzipTestGet(t, h, "deflate, gzip;q=0.8, br")
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding %q, want gzip", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary %q", got)
	}
	if rec.Header().Get("Content-Length") != "" || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("headers changed: %v", rec.Header())
	}
	if rec.Body.Len() >= len(want) {
		t.Fatalf("sent %d bytes for a %d byte body", rec.Body.Len(), len(want))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(zr)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("round trip differs (%v)", err)
	}
}

func TestCompressJSONLeavesAlone(t *testing.T) {
	large := map[string]string{"text": strings.Repeat("x", 4000)}
	jsonLarge := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, large) })
	for _, c := range []struct {
		name, accept string
		h            http.Handler
	}{
		{"not asked", "", jsonLarge},
		{"gzip refused with q=0", "gzip;q=0, deflate", jsonLarge},
		{"any refused with q=0", "*;q=0", jsonLarge},
		{"small body", "gzip", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })},
		{"no content", "gzip", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNoContent)
		})},
		{"not modified", "gzip", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusNotModified, large) })},
		{"already encoded", "gzip", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Encoding", "identity")
			writeJSON(w, 200, large)
		})},
		{"not JSON", "gzip", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("x", 4000)))
		})},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := gzipTestGet(t, c.h, c.accept)
			if got := rec.Header().Get("Content-Encoding"); got == "gzip" {
				t.Fatalf("compressed: Content-Encoding %q", got)
			}
			// Every JSON body written through writeJSON says it differs by
			// Accept-Encoding, compressed or not; nothing else is touched.
			vary := "Accept-Encoding"
			if c.name == "no content" || c.name == "not JSON" {
				vary = ""
			}
			if got := rec.Header().Get("Vary"); got != vary {
				t.Fatalf("Vary %q, want %q", got, vary)
			}
			if c.name != "no content" && c.name != "not modified" && !json.Valid(rec.Body.Bytes()) && c.name != "not JSON" {
				t.Fatalf("body not sent as it is: %q", rec.Body.String())
			}
		})
	}
	r := httptest.NewRequest("HEAD", "/x", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	compressJSON(jsonLarge).ServeHTTP(rec, r)
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("HEAD compressed")
	}
}

// A handler that wraps the response in a writer of its own, as the request
// log does, still has its JSON compressed.
func TestCompressJSONSeesThroughWrappers(t *testing.T) {
	large := map[string]string{"text": strings.Repeat("x", 4000)}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(&statusWriter{ResponseWriter: w}, 200, large)
	})
	if got := gzipTestGet(t, h, "gzip").Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding %q through a wrapper", got)
	}
}

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"": false, "gzip": true, "GZIP": true, "gzip, deflate, br": true, "br;q=1, gzip;q=0.5": true,
		"gzip;q=0": false, "gzip; q=0.000": false, "*": true, "*;q=0": false, "gzip;q=0, *": false, "*, gzip;q=0": false,
		"deflate": false, "x-gzip": false,
	} {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

// An event stream and a plain stream are sent as they are and each flush
// reaches the client before the handler returns; a hijack still works.
func TestCompressJSONKeepsStreams(t *testing.T) {
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		<-release
	})
	mux.HandleFunc("/plain", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "first\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Error(err)
		}
		<-release
	})
	mux.HandleFunc("/hijack", func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nhi")
		_ = buf.Flush()
	})
	srv := httptest.NewServer(compressJSON(mux))
	defer srv.Close()
	defer close(release)
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}, Timeout: 5 * time.Second}
	for path, first := range map[string]string{"/events": "data: one\n", "/plain": "first\n"} {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Header.Get("Content-Encoding") != "" || resp.Header.Get("Vary") != "" {
			t.Fatalf("%s: compressed", path)
		}
		line, err := bufio.NewReader(resp.Body).ReadString('\n')
		resp.Body.Close()
		if err != nil || line != first {
			t.Fatalf("%s: read %q before the handler returned (%v)", path, line, err)
		}
	}
	req, _ := http.NewRequest("GET", srv.URL+"/hijack", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "hi" {
		t.Fatalf("hijacked reply %q", body)
	}
}
