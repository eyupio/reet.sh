package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestChatPreviewUsesPrivateReplyFilterAndBrowserSessionOwnership(t *testing.T) {
	caps := chatCaps()
	caps.ACPPreviews = true
	f := launchAPIFixture(t, caps)
	data := []byte("\x89PNG\r\n\x1a\npreview snapshot")
	id := fmt.Sprintf("%x", sha256.Sum256(data))
	f.preview = func(in *terminalproto.ACPPreview) terminalproto.ACPPreviewResult {
		return terminalproto.ACPPreviewResult{ID: id, Offset: in.Offset, Size: int64(len(data)), MIME: "image/png", Data: data}
	}
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatal(code, body)
	}
	session := out["id"].(string)
	if !isPeerReply(&terminalproto.ACPPreviewResult{}) {
		t.Fatal("production private transport drops previews")
	}
	path := "/api/v2/terminal/sessions/" + session + "/chat/previews/" + id
	r := httptest.NewRequest("GET", path, nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != string(data) || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("preview: %d %v %q", w.Code, w.Header(), w.Body.String())
	}
	if code, _, _ := f.do(t, "", "GET", path, nil); code != 401 {
		t.Fatal("anonymous preview", code)
	}
	if code, _, _ := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions/unknown-session01234567/chat/previews/"+id, nil); code != 404 {
		t.Fatal("unknown session", code)
	}
}
