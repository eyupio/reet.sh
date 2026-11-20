package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

type historyReply struct {
	Revisions []store.SkillHistoryEntry `json:"revisions"`
}

// historyEnv is a written skill edited once: two versions, the edit current.
func historyEnv(t *testing.T) (reconcileEnv, string, []string) {
	t.Helper()
	e := newReconcileEnv(t)
	item := e.writtenSkill(t, "notes")
	li, err := e.st.LibraryItemFor(e.uid, item)
	if err != nil {
		t.Fatal(err)
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: notes\ndescription: Written in reet\n---\nEdited.\n")}}
	res, err := e.st.SetSkillContentFromFor(e.uid, item, files, store.PrivateSkillOrigin{Kind: "edit"}, li.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	return e, item, []string{li.RevisionID, res.Revision.ID}
}

func readHistory(t *testing.T, e reconcileEnv, item string) historyReply {
	t.Helper()
	code, raw := e.owner(t, "GET", "/api/v2/library-history/"+item, "")
	var h historyReply
	if code != 200 || json.Unmarshal([]byte(raw), &h) != nil {
		t.Fatalf("history: %d %s", code, raw)
	}
	return h
}

// The history lists a skill's versions newest first, the current one marked,
// to its owner only.
func TestSkillHistoryRoute(t *testing.T) {
	e, item, revs := historyEnv(t)
	h := readHistory(t, e, item)
	if len(h.Revisions) != 2 || h.Revisions[0].Revision != revs[1] || h.Revisions[0].Kind != "edit" || !h.Revisions[0].Current ||
		h.Revisions[1].Revision != revs[0] || h.Revisions[1].Current || h.Revisions[0].Files != 1 || h.Revisions[0].Digest == "" || h.Revisions[0].SavedAt == 0 {
		t.Fatalf("history: %+v", h)
	}
	other, err := e.st.CreateUser("history-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.CreateAPIToken(other.ID, "other", auth.HashToken("fixture-other")); err != nil {
		t.Fatal(err)
	}
	if code, raw := doClient(t, e.srv.Client(), e.srv, "GET", "/api/v2/library-history/"+item, "", "X-Reet-Token", "fixture-other"); code != 404 {
		t.Fatalf("another account read the history: %d %s", code, raw)
	}
	if code, raw := doClient(t, e.srv.Client(), e.srv, "POST", "/api/v2/library-history/"+item+"/restore", `{"revision":"`+revs[0]+`","head":"`+revs[1]+`"}`, "X-Reet-Token", "fixture-other"); code != 404 {
		t.Fatalf("another account put a version back: %d %s", code, raw)
	}
	if code, raw := e.owner(t, "GET", "/api/v2/library-history/nosuchitem", ""); code != 404 {
		t.Fatalf("unknown skill: %d %s", code, raw)
	}
}

// Put back makes a new current version of kind restore with the old files,
// which machines are given, and refuses a moved head, the current version and
// a version of another skill.
func TestSkillPutBackRoute(t *testing.T) {
	e, item, revs := historyEnv(t)
	restore := func(revision, head string) (int, string) {
		return e.owner(t, "POST", "/api/v2/library-history/"+item+"/restore", `{"revision":"`+revision+`","head":"`+head+`"}`)
	}
	if code, raw := restore(revs[0], revs[0]); code != 409 {
		t.Fatalf("a moved head: %d %s", code, raw)
	}
	if code, raw := restore(revs[1], revs[1]); code != 409 {
		t.Fatalf("the current version: %d %s", code, raw)
	}
	other := e.writtenSkill(t, "other")
	ol, err := e.st.LibraryItemFor(e.uid, other)
	if err != nil {
		t.Fatal(err)
	}
	if code, raw := restore(ol.RevisionID, revs[1]); code != 404 {
		t.Fatalf("another skill's version: %d %s", code, raw)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/library-history/"+item+"/restore", `{"revision":"`+revs[0]+`"}`); code != 400 {
		t.Fatalf("no head: %d %s", code, raw)
	}
	code, raw := restore(revs[0], revs[1])
	var out struct{ Revision string }
	if code != 200 || json.Unmarshal([]byte(raw), &out) != nil || out.Revision == "" {
		t.Fatalf("put back: %d %s", code, raw)
	}
	h := readHistory(t, e, item)
	if len(h.Revisions) != 3 || h.Revisions[0].Revision != out.Revision || h.Revisions[0].Kind != "restore" || h.Revisions[0].DerivedFrom != revs[0] ||
		!h.Revisions[0].Current || h.Revisions[0].Digest != h.Revisions[2].Digest {
		t.Fatalf("history after put back: %+v", h)
	}
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now().Add(store.SkillGrace))
	if err != nil {
		t.Fatal(err)
	}
	given := false
	for _, a := range d.Active {
		given = given || (a.Item == item && a.Revision == out.Revision)
	}
	if !given {
		t.Fatalf("machines are not given the version put back: %+v", d.Active)
	}
}
