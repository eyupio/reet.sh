//go:build linux || darwin

package acpagent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestAgentPreviewContentBlocksAndStreamedReferences(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nimage-data")
	raw, _ := json.Marshal([]any{map[string]any{"type": "content", "content": map[string]string{"type": "image", "mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(png)}}})
	pictures := contentPreviews(raw)
	if len(pictures) != 1 || !bytes.Equal(pictures[0].Data, png) || pictures[0].MIME != "image/png" {
		t.Fatalf("image lost: %+v", pictures)
	}
	raw = json.RawMessage(`{"type":"image","mimeType":"image/svg+xml","data":"PHN2Zz48L3N2Zz4="}`)
	if p := contentPreviews(raw); len(p) != 1 || p[0].Reason != "unsupported" {
		t.Fatalf("unsafe format: %+v", p)
	}
	var s Session
	if p := s.messagePreviews("msg", "Here is ![Screenshot]("); len(p) != 0 {
		t.Fatal(p)
	}
	if p := s.messagePreviews("msg", "/tmp/shot.png)"); len(p) != 1 || p[0].Path != "/tmp/shot.png" {
		t.Fatal(p)
	}
	if p := s.messagePreviews("msg", " and more text"); len(p) != 0 {
		t.Fatal("duplicate", p)
	}
	if p := s.messagePreviews("remote", "![external](https://example.test/secret.png)"); len(p) != 0 {
		t.Fatal("remote URL", p)
	}
}

func TestPreviewEventsKeepToolOutputAndMessageProgress(t *testing.T) {
	var events []Event
	s := Session{opts: Options{OnEvent: func(e Event) { events = append(events, e) }}, tools: map[string]string{}, toolStart: map[string]time.Time{}}
	s.handleUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call_update","toolCallId":"call-preview","kind":"read","status":"completed","rawInput":{"filePath":"/tmp/shot.png"},"content":[{"type":"content","content":{"type":"text","text":"Read image"}}]}}`))
	if len(events) != 2 || events[0].Kind != "tool" || events[0].Text != "Read image" || events[1].Kind != "preview" || events[1].Preview.Path != "/tmp/shot.png" {
		t.Fatalf("events: %+v", events)
	}
}

func TestPreviewTargetSurvivesSparseToolUpdates(t *testing.T) {
	var s Session
	if got := s.previewTarget("read-1", "read", "/tmp/screenshot.png", "in_progress"); got != "" {
		t.Fatal(got)
	}
	if got := s.previewTarget("read-1", "", "", "completed"); got != "/tmp/screenshot.png" {
		t.Fatal("sparse completion lost target", got)
	}
	if got := s.previewTarget("read-1", "", "", "completed"); got != "" {
		t.Fatal("completed tool target retained", got)
	}
}
