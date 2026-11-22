package api

import (
	"bufio"
	"compress/gzip"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipMinBytes is the smallest JSON body worth compressing: below about a
// kilobyte the gzip header and the CPU cost more than the bytes saved.
const gzipMinBytes = 1024

var gzipWriters = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// compressJSON gzips JSON responses for clients that accept it. Anything that
// is not JSON (event streams, downloads, uploads) passes straight through, and
// upgraded connections never see the wrapper.
func compressJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || r.Header.Get("Upgrade") != "" || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipJSONWriter{ResponseWriter: w}
		defer gw.finish()
		next.ServeHTTP(gw, r)
	})
}

// acceptsGzip reads Accept-Encoding: gzip, or else *, with a q above zero.
func acceptsGzip(header string) bool {
	star := false
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(part, ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "gzip" && name != "*" {
			continue
		}
		ok := true
		for _, p := range strings.Split(params, ";") {
			if k, v, found := strings.Cut(strings.TrimSpace(p), "="); found && strings.EqualFold(strings.TrimSpace(k), "q") {
				q, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
				ok = err == nil && q > 0
			}
		}
		if name == "gzip" {
			return ok
		}
		star = ok
	}
	return star
}

// gzipJSONWriter holds a JSON body until it reaches gzipMinBytes, then
// compresses it; a smaller one is sent as it is when the handler returns.
type gzipJSONWriter struct {
	http.ResponseWriter
	code      int
	wroteHead bool
	holding   bool
	buf       []byte
	gz        *gzip.Writer
}

func (w *gzipJSONWriter) WriteHeader(code int) {
	if w.wroteHead {
		return
	}
	if code < 200 {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.wroteHead, w.code = true, code
	h := w.Header()
	mt, _, _ := mime.ParseMediaType(h.Get("Content-Type"))
	isJSON := mt == "application/json" || strings.HasSuffix(mt, "+json")
	// A range (206) describes bytes of the uncompressed body, so it is left alone too.
	skip := code == http.StatusNoContent || code == http.StatusNotModified || code == http.StatusPartialContent
	if isJSON && h.Get("Content-Encoding") == "" && !skip {
		w.holding = true
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipJSONWriter) Write(b []byte) (int, error) {
	if !w.wroteHead {
		w.WriteHeader(http.StatusOK)
	}
	switch {
	case w.gz != nil:
		return w.gz.Write(b)
	case !w.holding:
		return w.ResponseWriter.Write(b)
	}
	w.buf = append(w.buf, b...)
	if len(w.buf) >= gzipMinBytes {
		if err := w.startGzip(); err != nil {
			return 0, err
		}
	}
	return len(b), nil
}

func (w *gzipJSONWriter) startGzip() error {
	h := w.Header()
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	h.Del("Content-Length")
	w.ResponseWriter.WriteHeader(w.code)
	w.gz = gzipWriters.Get().(*gzip.Writer)
	w.gz.Reset(w.ResponseWriter)
	w.holding = false
	_, err := w.gz.Write(w.buf)
	w.buf = nil
	return err
}

// sendHeld sends a small held body as it is.
func (w *gzipJSONWriter) sendHeld() {
	w.holding = false
	w.ResponseWriter.WriteHeader(w.code)
	if len(w.buf) > 0 {
		_, _ = w.ResponseWriter.Write(w.buf)
	}
	w.buf = nil
}

func (w *gzipJSONWriter) finish() {
	switch {
	case w.gz != nil:
		_ = w.gz.Close()
		w.gz.Reset(nil)
		gzipWriters.Put(w.gz)
		w.gz = nil
	case w.holding:
		w.sendHeld()
	}
}

// Flush sends what is held: a caller that flushes wants the bytes out now.
func (w *gzipJSONWriter) Flush() {
	_ = w.FlushError()
}

func (w *gzipJSONWriter) FlushError() error {
	if !w.wroteHead {
		w.WriteHeader(http.StatusOK)
	}
	switch {
	case w.gz != nil:
		if err := w.gz.Flush(); err != nil {
			return err
		}
	case w.holding:
		w.sendHeld()
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *gzipJSONWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *gzipJSONWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
