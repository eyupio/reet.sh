package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (f *browseAPI) post(t *testing.T, cookie, path, body string) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("Origin", "https://reet.example")
	r.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func pruneCaps() terminalproto.Capabilities {
	caps := browseCaps()
	caps.BranchPrune = true
	return caps
}

func TestBranchPrunePlanAndPruneFromTheMachine(t *testing.T) {
	f := browseAPIFixture(t, pruneCaps())
	oid := strings.Repeat("a", 40)
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		switch body.(type) {
		case terminalproto.BranchPrunePlan:
			return terminalproto.BranchPrunePlanResult{Current: "main", Default: "main", Base: "main", More: true,
				Branches: []terminalproto.BranchPruneBranch{{Name: "old", OID: oid}, {Name: "wip", OID: oid, Skip: terminalproto.BranchPruneNotMerged}}}
		case terminalproto.BranchPrune:
			return terminalproto.BranchPruneResult{Deleted: []string{"old"}, Skipped: []terminalproto.BranchPruneSkipped{{Name: "wip", Skip: terminalproto.BranchPruneNotMerged}}}
		}
		return nil
	})
	dir := url.QueryEscape("/home/tester/site")
	code, out, _ := f.get(t, f.cookie, "/api/v2/machines/"+f.agent+"/branches/prune?dir="+dir+"&base=main")
	if code != 200 || out["base"] != "main" || out["current"] != "main" || out["more"] != true {
		t.Fatalf("plan = %d %v", code, out)
	}
	branches, _ := out["branches"].([]any)
	if len(branches) != 2 {
		t.Fatalf("plan branches = %v", out["branches"])
	}
	if first, _ := branches[0].(map[string]any); first["name"] != "old" || first["skip"] != "" {
		t.Fatalf("first branch = %v", branches[0])
	}

	body := `{"dir":"/home/tester/site","base":"main","branches":[{"name":"old","oid":"` + oid + `"}]}`
	code, result := f.post(t, f.cookie, "/api/v2/machines/"+f.agent+"/branches/prune", body)
	if code != 200 {
		t.Fatalf("prune = %d %v", code, result)
	}
	deleted, _ := result["deleted"].([]any)
	if len(deleted) != 1 || deleted[0] != "old" {
		t.Fatalf("deleted = %v", result["deleted"])
	}

	events, err := f.st.ListEventsFor(f.uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	audited := false
	for _, e := range events {
		if e.Action == "branches-prune" && strings.Contains(e.Detail, "old") {
			audited = true
		}
	}
	if !audited {
		t.Fatal("no branches-prune audit row")
	}
}

func TestBranchPruneNeedsTheCapabilityOverHTTP(t *testing.T) {
	f := browseAPIFixture(t, browseCaps()) // no branch-prune-v1
	code, out, _ := f.get(t, f.cookie, "/api/v2/machines/"+f.agent+"/branches/prune?dir="+url.QueryEscape("/home/tester/site"))
	if code != http.StatusConflict || out["code"] != "branches_unsupported" {
		t.Fatalf("plan without the capability = %d %v", code, out)
	}
}

func TestBranchPruneRateLimit(t *testing.T) {
	f := browseAPIFixture(t, pruneCaps())
	oid := strings.Repeat("a", 40)
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BranchPrunePlanResult{Base: "main", Branches: []terminalproto.BranchPruneBranch{{Name: "old", OID: oid}}}
	})
	path := "/api/v2/machines/" + f.agent + "/branches/prune?dir=" + url.QueryEscape("/home/tester/site")
	for i := 0; i < 5; i++ {
		if code, out, _ := f.get(t, f.cookie, path); code != 200 {
			t.Fatalf("plan %d = %d %v", i, code, out)
		}
	}
	if code, out, _ := f.get(t, f.cookie, path); code != http.StatusTooManyRequests || out["code"] != "branches_rate_limited" {
		t.Fatalf("sixth plan = %d %v", code, out)
	}
}
