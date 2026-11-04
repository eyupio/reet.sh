package web

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

// gzipMinSize is the smallest body worth compressing. Below it the gzip header and the round trip through the encoder
// cost more than they save.
const gzipMinSize = 1024

// acceptsGzip reports whether the client will take a gzip body. It reads the q-values, because "gzip;q=0" is a refusal
// and a substring match would send a client that said so a body it asked not to have.
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(strings.Join(r.Header.Values("Accept-Encoding"), ","), ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			continue
		}
		q := 1.0
		if _, v, ok := strings.Cut(params, "="); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				q = f
			}
		}
		return q > 0
	}
	return false
}

// compressibleAsset is whether a built file is text a gzip body suits. Fonts and images are already compressed, and a
// second pass makes them larger.
func compressibleAsset(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".js", ".mjs", ".css", ".svg", ".json", ".html", ".webmanifest", ".txt", ".xml", ".map":
		return true
	}
	return false
}

// hashedAsset is whether Vite named the file after its content, which is what makes a year's cache safe: a changed file
// is a different address, and the old one is never asked for again.
func hashedAsset(name string) bool {
	return strings.HasPrefix(name, "assets/")
}

const immutableCache = "public, max-age=31536000, immutable"

// gzipBody compresses a body once, at the best ratio: the built files are served thousands of times and compressed once.
func gzipBody(body []byte) []byte {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = zw.Write(body)
	_ = zw.Close()
	return buf.Bytes()
}

// gzipCache holds each built file's gzip body, made the first time somebody asks for it. Compressing at startup would
// cost every start a few hundred milliseconds for files most visitors never fetch (the terminal workspace's chunk, for
// one); compressing per request would spend the CPU on the same bytes for every tab that opens.
type gzipCache struct{ m sync.Map }

func (c *gzipCache) body(name string, plain func() ([]byte, error)) ([]byte, error) {
	if v, ok := c.m.Load(name); ok {
		return v.([]byte), nil
	}
	raw, err := plain()
	if err != nil {
		return nil, err
	}
	body := gzipBody(raw)
	c.m.Store(name, body)
	return body, nil
}

// gzipWriters are the per-request encoders for the one body that is made per request, index.html.
var gzipWriters = sync.Pool{New: func() any {
	zw, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	return zw
}}

// writeMaybeGzip writes body, compressed when the client takes gzip and the body is worth it. Vary is said either way
// so a shared cache keeps the two bodies apart.
func writeMaybeGzip(w http.ResponseWriter, r *http.Request, body []byte) {
	writeMaybeGzipStatus(w, r, body, http.StatusOK)
}

// writeMaybeGzipStatus is writeMaybeGzip for a body whose status is not 200. The status is written after the encoding
// headers, because Go ignores header changes made once WriteHeader has been called.
func writeMaybeGzipStatus(w http.ResponseWriter, r *http.Request, body []byte, status int) {
	w.Header().Add("Vary", "Accept-Encoding")
	if len(body) < gzipMinSize || !acceptsGzip(r) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(status)
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(status)
	zw := gzipWriters.Get().(*gzip.Writer)
	zw.Reset(w)
	_, _ = zw.Write(body)
	_ = zw.Close()
	gzipWriters.Put(zw)
}
