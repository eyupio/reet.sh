package api

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// A scan upload that stalls while its body is still arriving (a laptop that
// slept mid-upload) must not hold the lock every other import and every machine
// merge waits on.
func TestAStalledImportUploadDoesNotHoldTheImportLock(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := newServer(st, meta.Fetcher{}, "")

	body, stall := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.importSkills(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/import", body))
	}()
	// Let the handler reach its read of the body.
	time.Sleep(100 * time.Millisecond)

	locked := make(chan bool, 1)
	go func() {
		ok := s.importMu.TryLock()
		if ok {
			s.importMu.Unlock()
		}
		locked <- ok
	}()
	if ok := <-locked; !ok {
		t.Error("the import lock is held while the upload body is still being read")
	}
	stall.Close()
	<-done
}
