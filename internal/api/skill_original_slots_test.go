package api

import (
	"testing"
)

// Update from original downloads as much as a background copy, so it shares the
// server's download slots: with every one taken it is told to wait (429), starts
// no download and gives back the account's own slot; once one is free it works.
func TestOriginalDownloadsShareTheServerWideSlots(t *testing.T) {
	e := newCopyEnv(t)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+findSkills+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	item := e.item(t)
	for range skillCopyFetches {
		skillCopySlots <- struct{}{}
	}
	held := skillCopyFetches
	free := func() {
		for ; held > 0; held-- {
			<-skillCopySlots
		}
	}
	t.Cleanup(free)
	before := e.fetch.callCount()
	for _, c := range []struct{ method, body string }{{"GET", ""}, {"POST", `{"digest":"d","revision":"r"}`}} {
		if code, body := do(t, e.srv, c.method, "/api/v2/skills/"+item+"/original", c.body); code != 429 {
			t.Fatalf("%s with every download slot taken: %d %s", c.method, code, body)
		}
	}
	if n := e.fetch.callCount(); n != before {
		t.Fatalf("%d downloads started with every slot taken", n-before)
	}
	e.api.s.originalMu.Lock()
	running := len(e.api.s.originalRunning)
	e.api.s.originalMu.Unlock()
	if running != 0 {
		t.Fatalf("the account's own slot was kept: %v", e.api.s.originalRunning)
	}
	<-skillCopySlots
	held--
	if code, body := do(t, e.srv, "GET", "/api/v2/skills/"+item+"/original", ""); code != 200 {
		t.Fatalf("with one slot free: %d %s", code, body)
	}
	if n := len(skillCopySlots); n != held {
		t.Fatalf("the download kept its slot: %d held, want %d", n, held)
	}
}
