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
	"strings"
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
