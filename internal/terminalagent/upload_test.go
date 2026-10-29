//go:build linux || darwin

package terminalagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const uploadTestSession = "session0123456789abcdef"

func uploadPayload() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), []byte("image bytes")...)
}

func uploadRequest(payload []byte) terminalproto.Upload {
	sum := sha256.Sum256(payload)
	return terminalproto.Upload{
		StreamToken: "token0123456789abcdef",
		Size:        int64(len(payload)),
		SHA256:      hex.EncodeToString(sum[:]),
		MIME:        "image/png",
	}
}

func uploadMessage(t *testing.T, req *terminalproto.Upload) terminalproto.Message {
	t.Helper()
	m := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: "upload-request-00001", SessionID: uploadTestSession, AttachmentID: "attachment0123456789ab", AttachmentGeneration: 1, Body: req}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	return m
}

func uploadHeaders() http.Header {
	h := http.Header{}
	h.Set(terminalproto.FeatureHeader, terminalproto.UploadFeature)
	return h
}

type uploadFrame struct {
	kind websocket.MessageType
	data []byte
	err  error
}

// fakeUploadConn serves canned frames and records what the connector writes.
type fakeUploadConn struct {
	frames    []uploadFrame
	sent      []uploadFrame
	readLimit int64
	closed    websocket.StatusCode
}

func (f *fakeUploadConn) Read(context.Context) (websocket.MessageType, []byte, error) {
	if len(f.frames) == 0 {
		return 0, nil, &websocket.CloseError{Code: websocket.StatusNormalClosure}
	}
	frame := f.frames[0]
	f.frames = f.frames[1:]
	return frame.kind, frame.data, frame.err
}

func (f *fakeUploadConn) Write(_ context.Context, kind websocket.MessageType, data []byte) error {
	f.sent = append(f.sent, uploadFrame{kind: kind, data: append([]byte(nil), data...)})
	return nil
}

func (f *fakeUploadConn) Close(code websocket.StatusCode, _ string) error {
	f.closed = code
	return nil
}

func (f *fakeUploadConn) CloseNow() error { return nil }

func (f *fakeUploadConn) SetReadLimit(n int64) { f.readLimit = n }

// fakeUploadSink reads everything the loop streams and answers with its own
// path or error.
type fakeUploadSink struct {
	path string
	err  error
	got  []byte
}

func (s *fakeUploadSink) WriteUpload(_ context.Context, _ string, _ terminalproto.Upload, src io.Reader) (string, error) {
	b, readErr := io.ReadAll(src)
	s.got = b
	if s.err != nil {
		return "", s.err
	}
	if readErr != nil {
		return "", readErr
	}
	return s.path, nil
}

func binaryFrames(data []byte) []uploadFrame {
	if len(data) == 0 {
		return nil
	}
	half := len(data) / 2
	if half == 0 {
		return []uploadFrame{{kind: websocket.MessageBinary, data: data}}
	}
	return []uploadFrame{
		{kind: websocket.MessageBinary, data: data[:half]},
		{kind: websocket.MessageBinary, data: data[half:]},
	}
}

func onlyResult(t *testing.T, got []uploadFrame) terminalproto.UploadResult {
	t.Helper()
	if len(got) != 1 || got[0].kind != websocket.MessageText {
		t.Fatalf("want exactly one text frame, got %+v", got)
	}
	result, err := terminalproto.DecodeUploadResult(got[0].data)
	if err != nil {
		t.Fatalf("result frame is invalid: %v (%s)", err, got[0].data)
	}
	return result
}

func TestServeUploadStreamsFramesAndReturnsTheStoredPath(t *testing.T) {
	payload := uploadPayload()
	req := uploadRequest(payload)
	conn := &fakeUploadConn{frames: binaryFrames(payload)}
	sink := &fakeUploadSink{path: "/home/u/.local/state/reet/uploads/session/img-0123.png"}

	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)

	result := serveUpload(t.Context(), conn, sink, uploadTestSession, req)

	if result != (terminalproto.UploadResult{Code: terminalproto.UploadOK, Path: sink.path}) {
		t.Fatalf("result: %+v", result)
	}
	if !bytes.Equal(sink.got, payload) {
		t.Fatal("bytes did not reach the sink")
	}
	if conn.readLimit != terminalproto.MaxBinaryBytes {
		t.Fatal("read limit", conn.readLimit)
	}
	if conn.closed != websocket.StatusNormalClosure {
		t.Fatal("stream did not close normally", conn.closed)
	}
	if got := onlyResult(t, conn.sent); got != result {
		t.Fatalf("result frame: %+v", got)
	}
	if strings.Contains(logs.String(), string(payload)) {
		t.Fatal("upload bytes were logged")
	}
}

func TestServeUploadMapsFrameErrors(t *testing.T) {
	payload := uploadPayload()
	req := uploadRequest(payload)
	for _, tc := range []struct {
		name   string
		frames []uploadFrame
		want   terminalproto.UploadCode
	}{
		{"text frame", []uploadFrame{{kind: websocket.MessageText, data: []byte("{}")}}, terminalproto.UploadType},
		{"empty binary", []uploadFrame{{kind: websocket.MessageBinary}}, terminalproto.UploadType},
		{"oversize frame", []uploadFrame{{kind: websocket.MessageBinary, data: make([]byte, terminalproto.MaxBinaryBytes+1)}}, terminalproto.UploadTooLarge},
		{"early close", binaryFrames(payload[:len(payload)/2]), terminalproto.UploadMismatch},
		{"wrong order", []uploadFrame{
			{kind: websocket.MessageBinary, data: payload[:2]},
			{kind: websocket.MessageText, data: []byte("{}")},
		}, terminalproto.UploadType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &fakeUploadConn{frames: tc.frames}
			sink := &fakeUploadSink{path: "/should/not/appear.png"}
			result := serveUpload(t.Context(), conn, sink, uploadTestSession, req)
			if result.Code != tc.want || result.Path != "" {
				t.Fatalf("result: %+v", result)
			}
			if got := onlyResult(t, conn.sent); got != result {
				t.Fatalf("result frame: %+v", got)
			}
			if bytes.Contains(conn.sent[0].data, []byte("should/not")) || bytes.Contains(conn.sent[0].data, []byte(req.SHA256)) {
				t.Fatal("result frame carried a path or hash", string(conn.sent[0].data))
			}
		})
	}
}

func TestServeUploadMapsHostErrors(t *testing.T) {
	payload := uploadPayload()
	req := uploadRequest(payload)
	for _, tc := range []struct {
		err  error
		want terminalproto.UploadCode
	}{
		{terminalhost.ErrUploadDisabled, terminalproto.UploadDisabled},
		{terminalhost.ErrUploadTooLarge, terminalproto.UploadTooLarge},
		{terminalhost.ErrUploadType, terminalproto.UploadType},
		{terminalhost.ErrUploadQuota, terminalproto.UploadQuota},
		{terminalhost.ErrUploadMismatch, terminalproto.UploadMismatch},
		{terminalhost.ErrUploadFailed, terminalproto.UploadFailed},
		{terminalproto.ErrInvalid, terminalproto.UploadType},
		{errors.New("boom"), terminalproto.UploadFailed},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			conn := &fakeUploadConn{frames: binaryFrames(payload)}
			sink := &fakeUploadSink{path: "/should/not/appear.png", err: tc.err}
			result := serveUpload(t.Context(), conn, sink, uploadTestSession, req)
			if result.Code != tc.want || result.Path != "" {
				t.Fatalf("result: %+v", result)
			}
			if got := onlyResult(t, conn.sent); got != result {
				t.Fatalf("result frame: %+v", got)
			}
		})
	}
}

func TestUploadCounterAllowsTwoAtATime(t *testing.T) {
	c := &uploadCounter{max: maxUploadsInFlight}
	if !c.acquire() || !c.acquire() {
		t.Fatal("two uploads must fit")
	}
	if c.acquire() {
		t.Fatal("a third upload was admitted")
	}
	c.release()
	if !c.acquire() {
		t.Fatal("a released slot was not reused")
	}
}

// uploadHandlerFixture gives the Factory a durable Running session without a
// tmux server: upload storage and the pre-stream checks only need the registry.
func uploadHandlerFixture(t *testing.T) (terminalhost.ManagerOptions, string) {
	t.Helper()
	if _, err := terminalhost.FindTmux(context.Background()); err != nil {
		t.Skip(err)
	}
	base, err := os.MkdirTemp("/tmp", "reet-upload-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	dir, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := terminalhost.NewLayout(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := terminalhost.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	id := uploadTestSession
	registry := map[string]any{
		"Version":      1,
		"EnrollmentID": "enrollment0123456789abcdef",
		"Sessions": map[string]any{
			id: map[string]any{
				"ID": id, "Title": "Uploads", "State": "running",
				"CreatedAt": 1, "UpdatedAt": 1, "CreateRequestID": "request0123456789abcdef",
			},
		},
		"Operations": map[string]any{
			"create:request0123456789abcdef": map[string]any{
				"requestId": "request0123456789abcdef",
				"sessionId": id,
				"kind":      "create",
				"create":    map[string]any{"title": "Uploads", "cols": 80, "rows": 24},
				"complete":  true,
			},
		},
	}
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Dir, "registry.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return terminalhost.ManagerOptions{
		EnrollmentID: "enrollment0123456789abcdef", Layout: layout, Home: home,
		Consent:       func() error { return nil },
		UploadConsent: func() error { return nil },
	}, id
}

type blockingUploadConn struct {
	payload []byte
	release <-chan struct{}
	sent    []uploadFrame
	mu      sync.Mutex
}

func (c *blockingUploadConn) Read(context.Context) (websocket.MessageType, []byte, error) {
	<-c.release
	if len(c.payload) == 0 {
		return 0, nil, &websocket.CloseError{Code: websocket.StatusNormalClosure}
	}
	data := c.payload
	c.payload = nil
	return websocket.MessageBinary, data, nil
}

func (c *blockingUploadConn) Write(_ context.Context, kind websocket.MessageType, data []byte) error {
	c.mu.Lock()
	c.sent = append(c.sent, uploadFrame{kind: kind, data: append([]byte(nil), data...)})
	c.mu.Unlock()
	return nil
}

func (c *blockingUploadConn) Close(websocket.StatusCode, string) error { return nil }
func (c *blockingUploadConn) CloseNow() error                          { return nil }
func (c *blockingUploadConn) SetReadLimit(int64)                       {}

func TestUploadHandlerHoldsTwoAndRefusesTheThird(t *testing.T) {
	opts, _ := uploadHandlerFixture(t)
	release := make(chan struct{})
	var dials atomic.Int32
	var mu sync.Mutex
	var conns []*blockingUploadConn
	original := dialUploadStream
	dialUploadStream = func(context.Context, *http.Client, http.Header, terminalproto.Message, string, string) (uploadConn, error) {
		dials.Add(1)
		conn := &blockingUploadConn{payload: uploadPayload(), release: release}
		mu.Lock()
		conns = append(conns, conn)
		mu.Unlock()
		return conn, nil
	}
	defer func() { dialUploadStream = original }()

	replies := make(chan terminalproto.Message, 8)
	handle, cleanup := Factory(opts)(t.Context(), http.DefaultClient, uploadHeaders(), 1, func(_ context.Context, m terminalproto.Message) error {
		replies <- m
		return nil
	})
	req := uploadRequest(uploadPayload())
	for range 2 {
		if err := handle(t.Context(), uploadMessage(t, &req)); err != nil {
			t.Fatal(err)
		}
	}
	if err := handle(t.Context(), uploadMessage(t, &req)); err != nil {
		t.Fatal(err)
	}
	seen := map[terminalproto.ResultCode]int{}
	for len(seen) < 3 || (seen[terminalproto.OK] != 2 || seen[terminalproto.UploadBusy] != 1) {
		select {
		case m := <-replies:
			result, ok := m.Body.(terminalproto.Result)
			if !ok {
				t.Fatalf("reply body %T", m.Body)
			}
			seen[result.Code]++
			if len(seen) == 1 && result.Code == terminalproto.UploadBusy {
				break
			}
			if seen[terminalproto.OK]+seen[terminalproto.UploadBusy] >= 3 {
				goto repliesDone
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("replies: %v", seen)
		}
	}
repliesDone:
	if seen[terminalproto.OK] != 2 || seen[terminalproto.UploadBusy] != 1 {
		t.Fatalf("replies: %v", seen)
	}
	if dials.Load() != 2 {
		t.Fatalf("dialed %d times, want 2", dials.Load())
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		done := true
		for _, conn := range conns {
			conn.mu.Lock()
			if len(conn.sent) == 0 {
				done = false
			}
			conn.mu.Unlock()
		}
		mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("transfers did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cleanup()
	mu.Lock()
	defer mu.Unlock()
	if len(conns) != 2 {
		t.Fatalf("connections: %d", len(conns))
	}
	for _, conn := range conns {
		conn.mu.Lock()
		sent := append([]uploadFrame(nil), conn.sent...)
		conn.mu.Unlock()
		if len(sent) != 1 || sent[0].kind != websocket.MessageText {
			t.Fatalf("result frames: %+v", sent)
		}
		var result terminalproto.UploadResult
		if err := json.Unmarshal(sent[0].data, &result); err != nil || result.Code != terminalproto.UploadOK || result.Path == "" {
			t.Fatalf("result: %+v %v", result, err)
		}
	}
}

func TestUploadRequiresNegotiatedFeature(t *testing.T) {
	got := make(chan terminalproto.Message, 1)
	handle, cleanup := Factory(terminalhost.ManagerOptions{})(t.Context(), http.DefaultClient, http.Header{}, 1, func(_ context.Context, m terminalproto.Message) error {
		got <- m
		return nil
	})
	defer cleanup()
	req := uploadRequest(uploadPayload())
	if err := handle(t.Context(), uploadMessage(t, &req)); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatalf("unnegotiated upload: %v", err)
	}
	select {
	case m := <-got:
		t.Fatalf("answered an unnegotiated upload: %+v", m)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestUploadRefusedBeforeAnyStreamWhenConsentIsOff(t *testing.T) {
	off := errors.New("off")
	for name, opts := range map[string]terminalhost.ManagerOptions{
		"no terminal consent": {},
		"terminal on, no upload switch": {
			Consent: func() error { return nil },
		},
		"terminal on, uploads off": {
			Consent:       func() error { return nil },
			UploadConsent: func() error { return off },
		},
		"terminal off, uploads on": {
			Consent:       func() error { return off },
			UploadConsent: func() error { return nil },
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := make(chan terminalproto.Message, 1)
			handle, cleanup := Factory(opts)(t.Context(), http.DefaultClient, uploadHeaders(), 1, func(_ context.Context, m terminalproto.Message) error {
				got <- m
				return nil
			})
			defer cleanup()
			req := uploadRequest(uploadPayload())
			if err := handle(t.Context(), uploadMessage(t, &req)); err != nil {
				t.Fatal(err)
			}
			select {
			case m := <-got:
				if r, ok := m.Body.(terminalproto.Result); !ok || r.Code != terminalproto.UploadsOff {
					t.Fatalf("reply: %+v", m.Body)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("no refusal")
			}
		})
	}
}

// A result frame never carries a path or hash on failure, whatever encoded it.
func TestUploadFailureResultFrameShape(t *testing.T) {
	payload := uploadPayload()
	req := uploadRequest(payload)
	conn := &fakeUploadConn{frames: binaryFrames(payload)}
	sink := &fakeUploadSink{err: terminalhost.ErrUploadQuota}
	result := serveUpload(t.Context(), conn, sink, uploadTestSession, req)
	frame := conn.sent[0]
	if result.Path != "" || frame.kind != websocket.MessageText {
		t.Fatalf("result: %+v", result)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(frame.data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["path"]; ok {
		t.Fatalf("failure frame has a path: %s", frame.data)
	}
	if bytes.Contains(frame.data, []byte(req.SHA256)) || bytes.Contains(frame.data, []byte(uploadTestSession)) {
		t.Fatalf("failure frame leaked metadata: %s", frame.data)
	}
}

func TestReadUploadFramesStopsAtDeclaredSize(t *testing.T) {
	payload := uploadPayload()
	conn := &fakeUploadConn{frames: []uploadFrame{
		{kind: websocket.MessageBinary, data: payload},
		{kind: websocket.MessageBinary, data: []byte("after the declared size")},
	}}
	var got bytes.Buffer
	if err := readUploadFrames(t.Context(), conn, &got, int64(len(payload))); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), payload) {
		t.Fatal("the declared bytes did not arrive")
	}
	if len(conn.frames) != 1 {
		t.Fatal("the reader waited for a close after the declared size")
	}
}

func TestReadUploadFramesRejectsAFrameOverTheDeclaredSize(t *testing.T) {
	payload := uploadPayload()
	conn := &fakeUploadConn{frames: []uploadFrame{{kind: websocket.MessageBinary, data: append(append([]byte(nil), payload...), 0)}}}
	err := readUploadFrames(t.Context(), conn, &bytes.Buffer{}, int64(len(payload)))
	if !errors.Is(err, errUploadFrameOverDeclared) {
		t.Fatalf("want a declared-size mismatch, got %v", err)
	}
}

func TestReadUploadFramesShortStreamFails(t *testing.T) {
	payload := uploadPayload()
	conn := &fakeUploadConn{frames: binaryFrames(payload[:len(payload)-1])}
	err := readUploadFrames(t.Context(), conn, &bytes.Buffer{}, int64(len(payload)))
	if !errors.Is(err, errUploadShort) {
		t.Fatalf("want a short stream error, got %v", err)
	}
}
