package api

import (
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

// Community's Save sends the status the person chose: Active, as preselected,
// or Archived.
func TestSaveFromCommunityUsesTheChosenStatus(t *testing.T) {
	e := newReconcileEnv(t)
	for input, status := range map[string]string{"a/b/kept-on": "active", "a/b/kept-off": "archived"} {
		if code, raw := e.owner(t, "POST", "/api/skills", `{"input":"`+input+`","status":"`+status+`"}`); code != 200 {
			t.Fatalf("save %s: %d %s", input, code, raw)
		}
	}
	names := e.libraryNames(t)
	if names["kept-on"].Status != "active" || names["kept-off"].Status != "archived" {
		t.Fatalf("library: %+v", names)
	}
	if code, raw := e.owner(t, "POST", "/api/skills", `{"input":"a/b/odd","status":"to_test"}`); code != 400 {
		t.Fatalf("an unknown status: %d %s", code, raw)
	}
	if _, ok := e.libraryNames(t)["odd"]; ok {
		t.Fatal("an unknown status still added the skill")
	}
}

// A list's Add all to my library sends the status the person chose.
func TestListAddAllUsesTheChosenStatus(t *testing.T) {
	e := newReconcileEnv(t)
	list, err := e.st.CreateList(e.uid, "Picks", "", "personal")
	if err != nil {
		t.Fatal(err)
	}
	// The skill is in the catalogue from someone else's library, not in this one.
	other, err := e.st.CreateUser("lister@example.test", "Lister", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ref.Parse("a/b/listed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.UpsertFor(other.ID, r, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	if err = e.st.AddListSkill(list.ID, r.ID, "", other.ID); err != nil {
		t.Fatal(err)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/add-to-shelf", `{"status":"archived"}`); code != 200 || !strings.Contains(raw, `"added":1`) {
		t.Fatalf("add all: %d %s", code, raw)
	}
	if got := e.libraryNames(t)["listed"]; got.Status != "archived" {
		t.Fatalf("listed: %+v", got)
	}
	if code, raw := e.owner(t, "POST", "/api/v2/lists/"+strconv.FormatInt(list.ID, 10)+"/add-to-shelf", `{"status":"parked"}`); code != 400 {
		t.Fatalf("an unknown status: %d %s", code, raw)
	}
}

// Home's Waiting for you reads the skills New on a machine without reading the
// whole Skills page.
func TestNewSkillsReadsWhatWaitsForKeep(t *testing.T) {
	e := newReconcileEnv(t)
	e.poll(t, "", true)
	if code, raw := e.owner(t, "GET", "/api/v2/skills/new", ""); code != 200 || raw != `{"newOnMachines":[]}`+"\n" && raw != `{"newOnMachines":[]}` {
		t.Fatalf("empty: %d %q", code, raw)
	}
	if code, raw := e.call(t, "/api/v2/sync/skills/report", skillReportBody(store.SkillReportEntry{Folder: "~/.agents/skills", Name: "cq", State: "unknown", Digest: strings.Repeat("c", 64)}), true); code != 200 {
		t.Fatalf("report: %d %s", code, raw)
	}
	code, raw := e.owner(t, "GET", "/api/v2/skills/new", "")
	if code != 200 || !strings.Contains(raw, `"name":"cq"`) || !strings.Contains(raw, `"host":"elitebook"`) || !strings.Contains(raw, `"seenAt":`) {
		t.Fatalf("new: %d %s", code, raw)
	}
}
