package api

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipMinBytes is the smallest JSON body worth compressing: below about a
// kilobyte the gzip header and the CPU cost more than the bytes saved.
const gzipMinBytes = 1024

// gzipWriters holds compressors at the fastest level. The point of the
// compression is the bytes it saves on the wire; BestSpeed gives most of that
// for a fraction of the CPU the default level spends on the request path.
var gzipWriters = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
	return w
}}

// compressJSON marks the response of a request that accepts gzip, so
// writeJSON can compress a large body. Nothing else is touched: the mark
// writes nothing itself, so streams, downloads and upgraded connections go
// out exactly as their handlers write them.
func compressJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || r.Header.Get("Upgrade") != "" || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(gzipAccepted{w}, r)
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

// gzipAccepted marks a response whose request accepts gzip. It overrides
// nothing that writes; flushing and hijacking reach the writer underneath.
type gzipAccepted struct{ http.ResponseWriter }

func (w gzipAccepted) Flush() { _ = w.FlushError() }

func (w gzipAccepted) FlushError() error {
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w gzipAccepted) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w gzipAccepted) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// takesGzip reports whether w carries the mark, under any wrappers that
// unwrap to it.
func takesGzip(w http.ResponseWriter) bool {
	for {
		switch v := w.(type) {
		case gzipAccepted:
			return true
		case interface{ Unwrap() http.ResponseWriter }:
			w = v.Unwrap()
		default:
			return false
		}
	}
}

// gzipJSON compresses a JSON body for a response marked as taking gzip,
// setting the headers that go with it. It answers the body to send: the
// same bytes when it is small, the status carries no body of its own, or
// the handler set an encoding already.
func gzipJSON(w http.ResponseWriter, code int, data []byte) []byte {
	h := w.Header()
	// The body differs by Accept-Encoding whether or not this one was compressed.
	h.Add("Vary", "Accept-Encoding")
	// A range (206) describes bytes of the uncompressed body, so it is left alone too.
	skip := code == http.StatusNoContent || code == http.StatusNotModified || code == http.StatusPartialContent
	if len(data) < gzipMinBytes || skip || h.Get("Content-Encoding") != "" || !takesGzip(w) {
		return data
	}
	var buf bytes.Buffer
	gz := gzipWriters.Get().(*gzip.Writer)
	defer gzipWriters.Put(gz)
	gz.Reset(&buf)
	_, err := gz.Write(data)
	if err == nil {
		err = gz.Close()
	}
	gz.Reset(io.Discard)
	if err != nil {
		return data
	}
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	return buf.Bytes()
}
