package objectstore

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type multipartFake struct {
	mu       sync.Mutex
	objects  map[string][]byte
	uploads  map[string]map[int][]byte
	lostOnce bool
	failPart int
}

func (f *multipartFake) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodPost && q.Has("uploads"):
		f.uploads["up"] = map[int][]byte{}
		fmt.Fprint(w, "<InitiateMultipartUploadResult><UploadId>up</UploadId></InitiateMultipartUploadResult>")
	case r.Method == http.MethodPut && q.Get("uploadId") != "":
		n, _ := strconv.Atoi(q.Get("partNumber"))
		if f.failPart == n {
			http.Error(w, "<Error><Code>InternalError</Code></Error>", http.StatusInternalServerError)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.uploads[q.Get("uploadId")][n] = body
		w.Header().Set("ETag", `"etag"`)
	case r.Method == http.MethodPost && q.Get("uploadId") != "":
		body, _ := io.ReadAll(r.Body)
		var complete struct {
			Parts []struct {
				PartNumber int    `xml:"PartNumber"`
				ETag       string `xml:"ETag"`
			} `xml:"Part"`
		}
		_ = xml.Unmarshal(body, &complete)
		var nums []int
		for n := range f.uploads[q.Get("uploadId")] {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		var whole []byte
		for _, n := range nums {
			whole = append(whole, f.uploads[q.Get("uploadId")][n]...)
		}
		delete(f.uploads, q.Get("uploadId"))
		f.objects[key] = whole
		if f.lostOnce {
			f.lostOnce = false
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		fmt.Fprint(w, "<CompleteMultipartUploadResult><Key>"+key+"</Key></CompleteMultipartUploadResult>")
	case r.Method == http.MethodDelete && q.Get("uploadId") != "":
		delete(f.uploads, q.Get("uploadId"))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodHead:
		if _, ok := f.objects[key]; ok {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	case r.Method == http.MethodGet:
		body, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	case r.Method == http.MethodGet && q.Get("list-type") == "2":
		prefix := q.Get("prefix")
		fmt.Fprint(w, "<ListBucketResult><IsTruncated>false</IsTruncated>")
		for k := range f.objects {
			if strings.HasPrefix(k, prefix) {
				fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size></Contents>", k, len(f.objects[k]))
			}
		}
		fmt.Fprint(w, "</ListBucketResult>")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newMultipartClient(t *testing.T, f *multipartFake) *Client {
	t.Helper()
	f.objects, f.uploads = map[string][]byte{}, map[string]map[int][]byte{}
	server := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(server.Close)
	c, err := New(Config{Endpoint: server.URL, Bucket: "bucket", AccessKey: "key", SecretKey: "secret", PathStyle: boolValue(true)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func boolValue(v bool) *bool { return &v }

func TestMultipartCopiesAndSettlesALostCompletionAnswer(t *testing.T) {
	oldLimit, oldPart := SinglePutLimit, MultipartPartSize
	SinglePutLimit, MultipartPartSize = 8, 4
	t.Cleanup(func() { SinglePutLimit, MultipartPartSize = oldLimit, oldPart })
	f := &multipartFake{lostOnce: true}
	c := newMultipartClient(t, f)
	body := []byte("a whole object split across parts")
	if err := c.PutObject(context.Background(), "p/instance/copy.gz.enc", body); err != nil {
		t.Fatalf("lost completion answer: %v", err)
	}
	f.mu.Lock()
	got := f.objects["p/instance/copy.gz.enc"]
	left := len(f.uploads)
	f.mu.Unlock()
	if !bytes.Equal(got, body) || left != 0 {
		t.Fatalf("object = %q, uploads left = %d", got, left)
	}
}

func TestAFailedMultipartUploadLeavesNoCopyAdvertised(t *testing.T) {
	oldLimit, oldPart := SinglePutLimit, MultipartPartSize
	SinglePutLimit, MultipartPartSize = 8, 4
	t.Cleanup(func() { SinglePutLimit, MultipartPartSize = oldLimit, oldPart })
	f := &multipartFake{failPart: 2}
	c := newMultipartClient(t, f)
	body := []byte("a whole object split across parts")
	if err := c.PutObject(context.Background(), "p/instance/copy.gz.enc", body); err == nil {
		t.Fatal("a failed part was reported as a copy")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, there := f.objects["p/instance/copy.gz.enc"]; there {
		t.Fatal("a failed upload advertised an object")
	}
	if len(f.uploads) != 0 {
		t.Fatal("a failed upload was not aborted")
	}
}
