package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// A note is kept up to 20,000 characters, the most the MCP tools take. It
// comes back on every list and is searched by every query, so a longer one is
// refused, on create and on patch, and the stored note is left as it was.
func TestAShelfNoteIsLimited(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	add := func(notes string) (int, string) {
		b, _ := json.Marshal(map[string]any{"input": findSkills, "notes": notes})
		return do(t, srv, "POST", "/api/skills", string(b))
	}
	patch := func(notes string) (int, string) {
		b, _ := json.Marshal(map[string]any{"id": "vercel-labs/skills/find-skills", "notes": notes})
		return do(t, srv, "PATCH", "/api/skill", string(b))
	}
	stored := func() string {
		skills := list(t, srv, "").Skills
		if len(skills) != 1 {
			t.Fatalf("%d skills listed", len(skills))
		}
		return skills[0].Notes
	}

	// Over the limit on create: refused, and nothing is added.
	if code, body := add(strings.Repeat("a", 20001)); code != 400 || !strings.Contains(body, "20000 characters") {
		t.Fatalf("an add with a note one over: %d %.200s", code, body)
	}
	if n := len(list(t, srv, "").Skills); n != 0 {
		t.Fatalf("a refused add still added %d skills", n)
	}

	// At the limit on create, counted in characters and not bytes.
	atLimit := strings.Repeat("é", 20000)
	if code, body := add(atLimit); code != 200 {
		t.Fatalf("an add with a note at the limit: %d %.200s", code, body)
	}
	if got := stored(); got != atLimit {
		t.Fatalf("the note at the limit was kept as %d bytes", len(got))
	}

	// Patch: one over is refused and leaves the note; at the limit passes.
	if code, body := patch(strings.Repeat("é", 20001)); code != 400 || !strings.Contains(body, "20000 characters") {
		t.Fatalf("a patch with a note one over: %d %.200s", code, body)
	}
	if got := stored(); got != atLimit {
		t.Fatalf("a refused patch changed the note to %d bytes", len(got))
	}
	if code, body := patch(strings.Repeat("b", 20000)); code != 200 {
		t.Fatalf("a patch with a note at the limit: %d %.200s", code, body)
	}
	if got := stored(); got != strings.Repeat("b", 20000) {
		t.Fatalf("the patched note was kept as %d bytes", len(got))
	}
	// Clearing a note is still allowed.
	if code, body := patch(""); code != 200 {
		t.Fatalf("clearing: %d %.200s", code, body)
	}
	if got := stored(); got != "" {
		t.Fatalf("cleared note is %.50q", got)
	}
}
