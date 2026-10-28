package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func uploadAPICaps(uploads, off bool) terminalproto.Capabilities {
	caps := launchCaps("claude")
	caps.Uploads = uploads
	caps.UploadsOff = off
	return caps
}

func uploadAPIData(n int) []byte {
	data := bytes.Repeat([]byte{0x42}, n)
	copy(data, []byte("\x89PNG\r\n\x1a\n"))
	return data
}

type uploadAPIFixture struct {
	srv       *Server
	st        *store.Store
	uid       int64
	agent     store.SyncAgent
	cookie    string
	sessionID string
	peer      *terminalbroker.Peer
	mu        sync.Mutex
	uploads   []terminalproto.Upload
	result    terminalproto.UploadResult
	data      *httptest.Server
}

func newUploadAPIFixture(t *testing.T, caps terminalproto.Capabilities) *uploadAPIFixture {
	t.Helper()
	srv, st, agent, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	f := &uploadAPIFixture{srv: srv, st: st, uid: uid, agent: agent, cookie: terminalLogin(t, st, uid)}
	f.result = terminalproto.UploadResult{Code: terminalproto.UploadOK, Path: "/home/tester/uploads/img-0123456789abcdef.png"}
	f.data = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		readCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		_, hello, err := c.Read(readCtx)
		cancel()
		if err != nil {
			return
		}
		h, err := terminalproto.DecodeStreamHello(hello)
		if err != nil {
			return
		}
		_ = srv.s.terminals.Bind(r.Context(), uid, agent.ID, h, c)
	}))
	t.Cleanup(f.data.Close)
	record, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	var p *terminalbroker.Peer
	p, err = srv.s.terminals.Connect(uid, agent.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Create:
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
		case terminalproto.Upload:
			f.mu.Lock()
			f.uploads = append(f.uploads, op)
			result := f.result
			f.mu.Unlock()
			go f.serve(m, op, result)
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		data, err := terminalproto.Encode(reply)
		if err != nil {
			return err
		}
		decoded, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		return p.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.peer = p
	session, err := srv.s.terminals.Create(t.Context(), uid, agent.ID, terminalbroker.ID(), terminalproto.Create{Title: "Attach", Cols: 80, Rows: 24, Tool: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	f.sessionID = session.ID
	return f
}

func (f *uploadAPIFixture) serve(m terminalproto.Message, req terminalproto.Upload, result terminalproto.UploadResult) {
	c, _, err := websocket.Dial(context.Background(), f.data.URL, nil)
	if err != nil {
		return
	}
	h := terminalproto.StreamHello{Version: terminalproto.Version, RequestID: m.RequestID, SessionID: m.SessionID, AttachmentID: m.AttachmentID, Generation: m.Generation, AttachmentGeneration: m.AttachmentGeneration, Token: req.StreamToken, Operation: "upload"}
	payload, _ := json.Marshal(h)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = c.Write(ctx, websocket.MessageText, payload)
	cancel()
	var total int64
	readCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	for total < req.Size {
		kind, data, err := c.Read(readCtx)
		if err != nil {
			stop()
			c.CloseNow()
			return
		}
		if kind != websocket.MessageBinary {
			stop()
			c.CloseNow()
			return
		}
		total += int64(len(data))
	}
	stop()
	frame, _ := json.Marshal(result)
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	_ = c.Write(ctx, websocket.MessageText, frame)
	cancel()
	_ = c.Close(websocket.StatusNormalClosure, "")
}

func (f *uploadAPIFixture) sent() []terminalproto.Upload {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Upload(nil), f.uploads...)
}

func (f *uploadAPIFixture) post(t *testing.T, cookie, target string, body []byte, contentType, origin string) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest("POST", target, bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, w.Body.String()
}

func (f *uploadAPIFixture) uploadTarget() string {
	return "/api/v2/terminal/sessions/" + f.sessionID + "/uploads"
}

func uploadEvents(t *testing.T, st *store.Store, uid int64) []store.Event {
	t.Helper()
	events, err := st.ListEventsFor(uid, 200, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Event
	for _, e := range events {
		if e.Action == "terminal-upload" {
			out = append(out, e)
		}
	}
	return out
}

func TestUploadRouteIsBlockedForViewersLegacyAuthAndOtherOrigins(t *testing.T) {
	f := newUploadAPIFixture(t, uploadAPICaps(true, false))
	body := uploadAPIData(64)
	viewer, err := f.st.CreateUser("upload-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, raw := f.post(t, terminalLogin(t, f.st, viewer.ID), f.uploadTarget(), body, "image/png", "https://reet.example"); code != 403 {
		t.Fatal("viewer uploaded", code, raw)
	}
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		f.srv.s.authMode = mode
		if code, _, raw := f.post(t, f.cookie, f.uploadTarget(), body, "image/png", "https://reet.example"); code != 403 {
			t.Fatal("legacy auth uploaded", mode, code, raw)
		}
	}
	f.srv.s.authMode = AuthAccounts
	for _, origin := range []string{"", "https://evil.example", "null"} {
		if code, _, raw := f.post(t, f.cookie, f.uploadTarget(), body, "image/png", origin); code != 403 {
			t.Fatal("origin accepted", origin, code, raw)
		}
	}
	if got := f.sent(); len(got) != 0 {
		t.Fatal("a blocked upload reached the machine", got)
	}
}

func TestUploadRouteIsScopedToTheOwner(t *testing.T) {
	f := newUploadAPIFixture(t, uploadAPICaps(true, false))
	other, err := f.st.CreateUser("other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	cookie := terminalLogin(t, f.st, other.ID)
	code, out, raw := f.post(t, cookie, f.uploadTarget(), uploadAPIData(64), "image/png", "https://reet.example")
	if code != 403 {
		t.Fatalf("another account reached the upload: %d %v %s", code, out, raw)
	}
	if got := f.sent(); len(got) != 0 {
		t.Fatal("a non-owner upload reached the machine", got)
	}
}

func TestUploadRejectsQueryAndBadBodies(t *testing.T) {
	f := newUploadAPIFixture(t, uploadAPICaps(true, false))
	target := f.uploadTarget()
	body := uploadAPIData(64)
	for name, tc := range map[string]struct {
		target      string
		body        []byte
		contentType string
		code        int
		errCode     string
	}{
		"query":        {target + "?size=1", body, "image/png", 400, ""},
		"empty":        {target, nil, "image/png", 400, ""},
		"not an image": {target, body, "text/plain", 415, "upload_type"},
		"svg":          {target, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), "image/png", 415, "upload_type"},
		"wrong magic":  {target, []byte{0xff, 0xd8, 0xff, 0xe0, 0, 1, 2, 3}, "image/png", 415, "upload_type"},
		"oversize":     {target, append(uploadAPIData(terminalproto.MaxUploadBytes), 0, 0), "image/png", 413, "upload_too_large"},
	} {
		t.Run(name, func(t *testing.T) {
			code, out, raw := f.post(t, f.cookie, tc.target, tc.body, tc.contentType, "https://reet.example")
			if code != tc.code {
				t.Fatalf("want %d, got %d: %s", tc.code, code, raw)
			}
			if tc.errCode != "" && out["code"] != tc.errCode {
				t.Fatalf("error code %v: %s", out["code"], raw)
			}
		})
	}
	if got := f.sent(); len(got) != 0 {
		t.Fatal("an invalid body reached the machine", got)
	}
}

func TestUploadMapsUnsupportedAndOffMachines(t *testing.T) {
	for name, tc := range map[string]struct {
		caps    terminalproto.Capabilities
		errCode string
		message string
	}{
		"unsupported": {uploadAPICaps(false, false), "upload_unsupported", "Update reet on that machine to attach images."},
		"off":         {uploadAPICaps(false, true), "uploads_off", "Image uploads are off on this machine."},
	} {
		t.Run(name, func(t *testing.T) {
			f := newUploadAPIFixture(t, tc.caps)
			code, out, raw := f.post(t, f.cookie, f.uploadTarget(), uploadAPIData(64), "image/png", "https://reet.example")
			if code != 409 || out["code"] != tc.errCode || out["error"] != tc.message {
				uploadFatal(t, "want a typed refusal", code, out, raw)
			}
			if got := f.sent(); len(got) != 0 {
				t.Fatal("a refused upload reached the machine", got)
			}
			if events := uploadEvents(t, f.st, f.uid); len(events) != 0 {
				t.Fatal("a refusal was audited", events)
			}
		})
	}
}

func uploadFatal(t *testing.T, reason string, code int, out map[string]any, raw string) {
	t.Helper()
	t.Fatalf("%s: %d %v %s", reason, code, out, raw)
}

func TestUploadRateLimitIsPerUser(t *testing.T) {
	f := newUploadAPIFixture(t, uploadAPICaps(false, false))
	body := uploadAPIData(64)
	for i := 0; i < uploadRateLimit; i++ {
		code, out, raw := f.post(t, f.cookie, f.uploadTarget(), body, "image/png", "https://reet.example")
		if code != 409 || out["code"] != "upload_unsupported" {
			t.Fatalf("request %d: %d %s", i+1, code, raw)
		}
	}
	code, out, raw := f.post(t, f.cookie, f.uploadTarget(), body, "image/png", "https://reet.example")
	if code != 429 || out["code"] != "upload_rate_limited" {
		t.Fatalf("want the rate limit answer, got %d %s", code, raw)
	}
}

func TestUploadLimitsBoundConcurrentUploads(t *testing.T) {
	var limits uploadLimits
	releaseOne, reason := limits.begin(1)
	if reason != "" {
		t.Fatal(reason)
	}
	releaseTwo, reason := limits.begin(1)
	if reason != "" {
		t.Fatal(reason)
	}
	if _, reason = limits.begin(1); reason != "upload_busy" {
		t.Fatalf("a third upload was admitted: %q", reason)
	}
	releaseOne()
	releaseThree, reason := limits.begin(1)
	if reason != "" {
		t.Fatal(reason)
	}
	releaseTwo()
	releaseThree()
}

func TestUploadLimitsRateIsPerUser(t *testing.T) {
	var limits uploadLimits
	for i := 0; i < uploadRateLimit; i++ {
		release, reason := limits.begin(2)
		if reason != "" {
			t.Fatalf("request %d: %s", i+1, reason)
		}
		release()
	}
	if _, reason := limits.begin(2); reason != "upload_rate_limited" {
		t.Fatalf("want the rate limit, got %q", reason)
	}
	release, reason := limits.begin(3)
	if reason != "" {
		t.Fatalf("another account was limited: %s", reason)
	}
	release()
}

func TestUploadHappyPathAuditsMetadataOnly(t *testing.T) {
	f := newUploadAPIFixture(t, uploadAPICaps(true, false))
	path := "/home/tester/My Images/img-0123456789abcdef.png"
	f.result = terminalproto.UploadResult{Code: terminalproto.UploadOK, Path: path}
	payload := uploadAPIData(200)
	code, out, raw := f.post(t, f.cookie, f.uploadTarget(), payload, "image/png", "https://reet.example")
	if code != 200 || out["path"] != path || out["reference"] != "'"+path+"'" {
		t.Fatalf("upload: %d %v %s", code, out, raw)
	}
	if got := f.sent(); len(got) != 1 {
		t.Fatalf("want one machine request, got %d", len(got))
	}
	events := uploadEvents(t, f.st, f.uid)
	if len(events) != 1 {
		t.Fatalf("want one audit row, got %d", len(events))
	}
	var detail map[string]string
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["machine"] != f.agent.ID || detail["host"] != "laptop" || detail["tool"] != "claude" || detail["mime"] != "image/png" || detail["size"] != "<256 KiB" {
		t.Fatalf("audit detail: %v", detail)
	}
	sum := sha256.Sum256(payload)
	for _, secret := range []string{path, "img-", hex.EncodeToString(sum[:])} {
		if strings.Contains(events[0].Detail, secret) {
			t.Fatalf("audit carried %q: %s", secret, events[0].Detail)
		}
	}
}

func TestUploadReferenceQuotesOnlyWhenNeeded(t *testing.T) {
	for in, want := range map[string]string{
		"/home/u/img-1.png":             "/home/u/img-1.png",
		"/home/u/a b.png":               "'/home/u/a b.png'",
		"/home/u/o'neil.png":            `'/home/u/o'\''neil.png'`,
		"/home/u/a:b@c%2B+d.png":        "/home/u/a:b@c%2B+d.png",
		"/home/u/a;rm -rf.png":          "'/home/u/a;rm -rf.png'",
		"relative path with spaces.png": "'relative path with spaces.png'",
	} {
		if got := uploadReference(in); got != want {
			t.Fatalf("%q: want %q, got %q", in, want, got)
		}
	}
}

func TestUploadSizeBuckets(t *testing.T) {
	for n, want := range map[int]string{
		1:                            "<256 KiB",
		256<<10 - 1:                  "<256 KiB",
		256 << 10:                    "<1 MiB",
		1<<20 - 1:                    "<1 MiB",
		1 << 20:                      "<=5 MiB",
		terminalproto.MaxUploadBytes: "<=5 MiB",
	} {
		if got := uploadSizeBucket(n); got != want {
			t.Fatalf("%d: want %q, got %q", n, want, got)
		}
	}
}
