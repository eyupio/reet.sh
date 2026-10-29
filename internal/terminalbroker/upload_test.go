package terminalbroker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func uploadCaps(t *testing.T, b *Broker, p *Peer, uid int64, agent string, uploads, off bool) {
	t.Helper()
	caps, err := b.st.RemoteCapabilitiesFor(uid, agent)
	if err != nil {
		t.Fatal(err)
	}
	caps.Capabilities.Uploads = uploads
	caps.Capabilities.UploadsOff = off
	if err := b.st.UpdateRemoteCapabilitiesFor(uid, agent, caps.InstanceID, p.Generation, caps.Capabilities); err != nil {
		t.Fatal(err)
	}
}

func uploadPayload(n int) []byte {
	data := bytes.Repeat([]byte{0x42}, n)
	copy(data, []byte("\x89PNG\r\n\x1a\n"))
	return data
}

func uploadSession(t *testing.T, b *Broker, uid int64, agent string) string {
	t.Helper()
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Upload", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

type uploadRecorder struct {
	mu       sync.Mutex
	uploads  []terminalproto.Upload
	messages []terminalproto.Message
	hellos   []terminalproto.StreamHello
	frames   [][]byte
}

func (r *uploadRecorder) addUpload(m terminalproto.Message, u terminalproto.Upload) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.uploads = append(r.uploads, u)
	r.messages = append(r.messages, m)
}

func (r *uploadRecorder) addHello(h terminalproto.StreamHello) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hellos = append(r.hellos, h)
}

func (r *uploadRecorder) addFrame(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, append([]byte(nil), data...))
}

func (r *uploadRecorder) snapshot() ([]terminalproto.Upload, []terminalproto.Message, []terminalproto.StreamHello, [][]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]terminalproto.Upload(nil), r.uploads...), append([]terminalproto.Message(nil), r.messages...), append([]terminalproto.StreamHello(nil), r.hellos...), append([][]byte(nil), r.frames...)
}

type uploadBehave func(ctx context.Context, c *websocket.Conn, req terminalproto.Upload, rec *uploadRecorder)

// attachUploadMachine records every upload the broker sends. When dial is
// true it also opens the reverse stream (mirroring a connector) and hands the
// connection to behave once the hello is written.
func attachUploadMachine(t *testing.T, b *Broker, p *Peer, uid int64, agent string, dial bool, behave uploadBehave) *uploadRecorder {
	t.Helper()
	rec := &uploadRecorder{}
	var dataServer *httptest.Server
	if dial {
		dataServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer c.CloseNow()
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			_, data, err := c.Read(ctx)
			cancel()
			if err != nil {
				return
			}
			h, err := terminalproto.DecodeStreamHello(data)
			if err != nil {
				return
			}
			rec.addHello(h)
			_ = b.Bind(r.Context(), uid, agent, h, c)
		}))
		t.Cleanup(dataServer.Close)
	}
	original := p.send
	p.send = func(ctx context.Context, m terminalproto.Message) error {
		up, ok := m.Body.(terminalproto.Upload)
		if !ok {
			return original(ctx, m)
		}
		rec.addUpload(m, up)
		if !dial {
			return original(ctx, m)
		}
		go func() {
			c, _, err := websocket.Dial(context.Background(), dataServer.URL, nil)
			if err != nil {
				return
			}
			h := terminalproto.StreamHello{Version: terminalproto.Version, RequestID: m.RequestID, SessionID: m.SessionID, AttachmentID: m.AttachmentID, Generation: m.Generation, AttachmentGeneration: m.AttachmentGeneration, Token: up.StreamToken, Operation: "upload"}
			payload, _ := json.Marshal(h)
			writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = c.Write(writeCtx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				c.CloseNow()
				return
			}
			if behave != nil {
				behave(context.Background(), c, up, rec)
			}
		}()
		return original(ctx, m)
	}
	return rec
}

func readDeclaredUpload(ctx context.Context, c *websocket.Conn, size int64, rec *uploadRecorder) bool {
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var total int64
	for total < size {
		kind, data, err := c.Read(readCtx)
		if err != nil {
			return false
		}
		if kind != websocket.MessageBinary {
			return false
		}
		rec.addFrame(data)
		total += int64(len(data))
	}
	return total == size
}

func writeUploadResult(c *websocket.Conn, result terminalproto.UploadResult) {
	data, _ := json.Marshal(result)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.Write(ctx, websocket.MessageText, data)
	_ = c.Close(websocket.StatusNormalClosure, "")
}

func TestUploadRefusesPeersWithoutTheCapabilityBeforeSending(t *testing.T) {
	for name, tc := range map[string]struct {
		uploads, off bool
		want         error
	}{
		"old peer":     {want: ErrUploadUnsupported},
		"switched off": {off: true, want: ErrUploadsOff},
	} {
		t.Run(name, func(t *testing.T) {
			b, p, uid, agent := fixture(t)
			uploadCaps(t, b, p, uid, agent, tc.uploads, tc.off)
			session := uploadSession(t, b, uid, agent)
			rec := attachUploadMachine(t, b, p, uid, agent, false, nil)
			path, err := b.Upload(t.Context(), uid, session, uploadPayload(64), "image/png")
			if !errors.Is(err, tc.want) || path != "" {
				t.Fatalf("want %v, got %q, %v", tc.want, path, err)
			}
			if uploads, _, _, _ := rec.snapshot(); len(uploads) != 0 {
				t.Fatalf("a request reached a peer without the capability: %+v", uploads)
			}
		})
	}
}

func TestUploadSendsOneExactRequestAndBoundedFrames(t *testing.T) {
	b, p, uid, agent := fixture(t)
	uploadCaps(t, b, p, uid, agent, true, false)
	session := uploadSession(t, b, uid, agent)
	path := "/home/u/.local/state/reet/uploads/session/img-0123.png"
	rec := attachUploadMachine(t, b, p, uid, agent, true, func(ctx context.Context, c *websocket.Conn, req terminalproto.Upload, rec *uploadRecorder) {
		if readDeclaredUpload(ctx, c, req.Size, rec) {
			writeUploadResult(c, terminalproto.UploadResult{Code: terminalproto.UploadOK, Path: path})
		}
	})
	var mu sync.Mutex
	var input int64
	p.SetTrafficObserver(func(in, _ int64, _ time.Time) {
		mu.Lock()
		input += in
		mu.Unlock()
	})
	payload := uploadPayload(70 << 10)
	got, err := b.Upload(t.Context(), uid, session, payload, "image/png")
	if err != nil || got != path {
		t.Fatalf("upload: %q, %v", got, err)
	}
	uploads, messages, hellos, frames := rec.snapshot()
	if len(uploads) != 1 || len(messages) != 1 {
		t.Fatalf("want exactly one request, got %d", len(uploads))
	}
	req := uploads[0]
	sum := sha256.Sum256(payload)
	if req.Size != int64(len(payload)) || req.MIME != "image/png" || req.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("request fields: %+v", req)
	}
	if len(hellos) != 1 || hellos[0].Operation != "upload" || hellos[0].Token != req.StreamToken {
		t.Fatalf("hello: %+v", hellos)
	}
	var joined []byte
	for _, frame := range frames {
		if len(frame) == 0 || len(frame) > terminalproto.MaxBinaryBytes {
			t.Fatalf("frame size %d", len(frame))
		}
		joined = append(joined, frame...)
	}
	if !bytes.Equal(joined, payload) {
		t.Fatal("the machine did not read the declared bytes")
	}
	mu.Lock()
	defer mu.Unlock()
	if input != int64(len(payload)) {
		t.Fatalf("traffic accounting: %d", input)
	}
}

func TestUploadMapsResultCodes(t *testing.T) {
	for name, tc := range map[string]struct {
		result terminalproto.UploadResult
		want   error
	}{
		"disabled":  {terminalproto.UploadResult{Code: terminalproto.UploadDisabled}, ErrUploadsOff},
		"too large": {terminalproto.UploadResult{Code: terminalproto.UploadTooLarge}, ErrUploadTooLarge},
		"type":      {terminalproto.UploadResult{Code: terminalproto.UploadType}, ErrUploadType},
		"quota":     {terminalproto.UploadResult{Code: terminalproto.UploadQuota}, ErrUploadQuota},
		"mismatch":  {terminalproto.UploadResult{Code: terminalproto.UploadMismatch}, ErrUploadMismatch},
		"conflict":  {terminalproto.UploadResult{Code: terminalproto.UploadConflict}, ErrUploadBusy},
		"failed":    {terminalproto.UploadResult{Code: terminalproto.UploadFailed}, ErrUploadFailed},
	} {
		t.Run(name, func(t *testing.T) {
			b, p, uid, agent := fixture(t)
			uploadCaps(t, b, p, uid, agent, true, false)
			session := uploadSession(t, b, uid, agent)
			attachUploadMachine(t, b, p, uid, agent, true, func(ctx context.Context, c *websocket.Conn, req terminalproto.Upload, rec *uploadRecorder) {
				if readDeclaredUpload(ctx, c, req.Size, rec) {
					writeUploadResult(c, tc.result)
				}
			})
			path, err := b.Upload(t.Context(), uid, session, uploadPayload(32), "image/png")
			if !errors.Is(err, tc.want) || path != "" {
				t.Fatalf("want %v, got %q, %v", tc.want, path, err)
			}
		})
	}
}

func TestUploadMapsControlRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		code terminalproto.ResultCode
		want error
	}{
		"uploads off": {terminalproto.UploadsOff, ErrUploadsOff},
		"busy":        {terminalproto.UploadBusy, ErrUploadBusy},
		"not found":   {terminalproto.NotFound, ErrDenied},
		"denied":      {terminalproto.Denied, ErrDenied},
		"failed":      {terminalproto.Failed, ErrUploadFailed},
	} {
		t.Run(name, func(t *testing.T) {
			b, p, uid, agent := fixture(t)
			uploadCaps(t, b, p, uid, agent, true, false)
			session := uploadSession(t, b, uid, agent)
			rec := attachUploadMachine(t, b, p, uid, agent, false, nil)
			original := p.send
			p.send = func(ctx context.Context, m terminalproto.Message) error {
				up, ok := m.Body.(terminalproto.Upload)
				if !ok {
					return original(ctx, m)
				}
				rec.addUpload(m, up)
				reply := m
				reply.Body = &terminalproto.Result{Code: tc.code}
				return p.Receive(reply)
			}
			path, err := b.Upload(t.Context(), uid, session, uploadPayload(32), "image/png")
			if !errors.Is(err, tc.want) || path != "" {
				t.Fatalf("want %v, got %q, %v", tc.want, path, err)
			}
			if uploads, _, _, _ := rec.snapshot(); len(uploads) != 1 {
				t.Fatalf("want one control send, got %d", len(uploads))
			}
		})
	}
}

func TestUploadWithoutAResultFailsOnce(t *testing.T) {
	b, p, uid, agent := fixture(t)
	uploadCaps(t, b, p, uid, agent, true, false)
	session := uploadSession(t, b, uid, agent)
	rec := attachUploadMachine(t, b, p, uid, agent, true, func(ctx context.Context, c *websocket.Conn, req terminalproto.Upload, rec *uploadRecorder) {
		if readDeclaredUpload(ctx, c, req.Size, rec) {
			_ = c.Close(websocket.StatusNormalClosure, "")
		}
	})
	path, err := b.Upload(t.Context(), uid, session, uploadPayload(32), "image/png")
	if !errors.Is(err, ErrUploadFailed) || path != "" {
		t.Fatalf("got %q, %v", path, err)
	}
	if uploads, _, _, _ := rec.snapshot(); len(uploads) != 1 {
		t.Fatalf("an upload was replayed: %d", len(uploads))
	}
}

func TestUploadRejectsAnOversizeResultFrame(t *testing.T) {
	b, p, uid, agent := fixture(t)
	uploadCaps(t, b, p, uid, agent, true, false)
	session := uploadSession(t, b, uid, agent)
	attachUploadMachine(t, b, p, uid, agent, true, func(ctx context.Context, c *websocket.Conn, req terminalproto.Upload, rec *uploadRecorder) {
		if !readDeclaredUpload(ctx, c, req.Size, rec) {
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = c.Write(ctx, websocket.MessageText, bytes.Repeat([]byte("x"), terminalproto.MaxControlBytes+1))
		_ = c.Close(websocket.StatusNormalClosure, "")
	})
	path, err := b.Upload(t.Context(), uid, session, uploadPayload(32), "image/png")
	if !errors.Is(err, ErrUploadFailed) || path != "" {
		t.Fatalf("got %q, %v", path, err)
	}
}

func TestUploadTimesOutAndDropsTheLateStream(t *testing.T) {
	old := uploadTransferTimeout
	uploadTransferTimeout = 150 * time.Millisecond
	defer func() { uploadTransferTimeout = old }()

	b, p, uid, agent := fixture(t)
	uploadCaps(t, b, p, uid, agent, true, false)
	session := uploadSession(t, b, uid, agent)
	rec := attachUploadMachine(t, b, p, uid, agent, false, nil)
	path, err := b.Upload(t.Context(), uid, session, uploadPayload(32), "image/png")
	if !errors.Is(err, ErrUploadTimeout) || path != "" {
		t.Fatalf("got %q, %v", path, err)
	}
	uploads, messages, _, _ := rec.snapshot()
	if len(uploads) != 1 {
		t.Fatalf("an upload was replayed: %d", len(uploads))
	}
	// The connector's stream arrives after the controller gave up. Binding it
	// is refused, and nothing is replayed.
	m := messages[0]
	h := terminalproto.StreamHello{Version: terminalproto.Version, RequestID: m.RequestID, SessionID: m.SessionID, AttachmentID: m.AttachmentID, Generation: m.Generation, AttachmentGeneration: m.AttachmentGeneration, Token: uploads[0].StreamToken, Operation: "upload"}
	if err := b.Bind(t.Context(), uid, agent, h, nil); !errors.Is(err, ErrDenied) {
		t.Fatalf("a late stream was bound: %v", err)
	}
}

func TestUploadDisconnectIsTyped(t *testing.T) {
	b, p, uid, agent := fixture(t)
	uploadCaps(t, b, p, uid, agent, true, false)
	session := uploadSession(t, b, uid, agent)
	rec := attachUploadMachine(t, b, p, uid, agent, false, nil)
	done := make(chan error, 1)
	go func() {
		_, err := b.Upload(t.Context(), uid, session, uploadPayload(32), "image/png")
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		uploads, _, _, _ := rec.snapshot()
		if len(uploads) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the upload was never sent")
		}
		time.Sleep(time.Millisecond)
	}
	p.Close()
	select {
	case err := <-done:
		if !errors.Is(err, ErrOffline) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a disconnect left the upload waiting")
	}
}

func TestUploadTimesOutWaitingForTheResult(t *testing.T) {
	old := uploadTransferTimeout
	uploadTransferTimeout = 150 * time.Millisecond
	defer func() { uploadTransferTimeout = old }()

	b, p, uid, agent := fixture(t)
	uploadCaps(t, b, p, uid, agent, true, false)
	session := uploadSession(t, b, uid, agent)
	late := make(chan struct{})
	rec := attachUploadMachine(t, b, p, uid, agent, true, func(ctx context.Context, c *websocket.Conn, req terminalproto.Upload, rec *uploadRecorder) {
		if !readDeclaredUpload(ctx, c, req.Size, rec) {
			close(late)
			return
		}
		time.Sleep(400 * time.Millisecond)
		writeUploadResult(c, terminalproto.UploadResult{Code: terminalproto.UploadOK, Path: "/late.png"})
		close(late)
	})
	path, err := b.Upload(t.Context(), uid, session, uploadPayload(32), "image/png")
	if !errors.Is(err, ErrUploadTimeout) || path != "" {
		t.Fatalf("got %q, %v", path, err)
	}
	<-late
	if uploads, _, _, _ := rec.snapshot(); len(uploads) != 1 {
		t.Fatalf("a late result caused a replay: %d", len(uploads))
	}
}
