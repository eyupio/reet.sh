package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

const workOrderTestText = "---\nreet: work-order/1\nobjective: Ship CSV export.\n%s---\n## Pipeline: Export API\nAdd the export.\n"

func workOrderText(header string) string {
	return strings.Replace(workOrderTestText, "%s", header, 1)
}

type workOrderAnswer struct {
	Order    *struct{ Sessions []json.RawMessage } `json:"order"`
	Problems []struct {
		Line int
		Text string
	} `json:"problems"`
	ProjectID string   `json:"projectId"`
	PathwayID string   `json:"pathwayId"`
	Notes     []string `json:"notes"`
}

func checkWorkOrder(t *testing.T, client *http.Client, srv *httptest.Server, text string) (int, workOrderAnswer, string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"text": text})
	code, body := gafferClient(t, client, srv, "POST", "/api/v2/gaffer/work-orders/check", string(raw))
	var a workOrderAnswer
	_ = json.Unmarshal([]byte(body), &a)
	return code, a, body
}

func runningOutcome(t *testing.T, st *store.Store, uid int64, id string) store.GafferOutcome {
	t.Helper()
	o, err := st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: id, Objective: "Ship CSV export.", AgentID: "machine-1", ProjectID: "project-1", Dir: "/work/billing", Tool: "opencode", MaxReviews: 8, Hours: 24,
		Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Export API", Prompt: "Add the export.", RequestID: strings.Repeat("c", 32),
			Stages: []store.GafferOutcomeStage{{Kind: "code", Tool: "opencode", Tier: "standard"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	o, err = st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State = "following", "following"
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestWorkOrderCheckReadsTheOrderAndLooksUpNames(t *testing.T) {
	st, srv, owner, project, pathway := pathwayStart(t)
	_ = project
	_ = pathway
	if _, err := st.CreateFolderProjectFor(1, store.FolderProject{Host: "m", Dir: "/work/api", Name: "Reports-API"}); err != nil {
		t.Fatal(err)
	}
	code, a, body := checkWorkOrder(t, owner, srv, workOrderText(""))
	if code != 200 || len(a.Problems) != 0 || a.Order == nil || len(a.Order.Sessions) != 1 || a.ProjectID != "" || len(a.Notes) != 0 {
		t.Fatalf("clean: %d %s", code, body)
	}
	code, a, body = checkWorkOrder(t, owner, srv, workOrderText("project: reports-api\n"))
	if code != 200 || a.ProjectID == "" || len(a.Notes) != 0 {
		t.Fatalf("known project: %d %s", code, body)
	}
	code, a, body = checkWorkOrder(t, owner, srv, workOrderText("project: Nowhere\n"))
	if code != 200 || a.ProjectID != "" || len(a.Notes) != 1 || a.Order == nil {
		t.Fatalf("unknown project: %d %s", code, body)
	}
	code, a, body = checkWorkOrder(t, owner, srv, workOrderText("pathway: careful\n"))
	if code != 200 || a.PathwayID == "" || len(a.Problems) != 0 {
		t.Fatalf("known pathway: %d %s", code, body)
	}
	code, a, body = checkWorkOrder(t, owner, srv, workOrderText("pathway: Missing\n"))
	if code != 200 || a.PathwayID != "" || len(a.Problems) != 1 || a.Order != nil {
		t.Fatalf("unknown pathway: %d %s", code, body)
	}
	code, a, body = checkWorkOrder(t, owner, srv, "not a work order")
	if code != 200 || a.Order != nil || len(a.Problems) != 1 || !strings.Contains(body, `"order":null`) {
		t.Fatalf("parser problem: %d %s", code, body)
	}
}

func TestWorkOrderCheckContinuesNeedsARunningOutcomeOfTheCaller(t *testing.T) {
	st, srv, owner := templateServer(t)
	password, _ := auth.HashPassword("long-enough-password")
	user, err := st.CreateUser("member@example.test", "member", "member", password)
	if err != nil {
		t.Fatal(err)
	}
	member := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, member, srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+user.Email+`","password":"long-enough-password"}`); code != 200 {
		t.Fatal(code)
	}
	mine := runningOutcome(t, st, 1, strings.Repeat("a", 32))
	theirs := runningOutcome(t, st, user.ID, strings.Repeat("b", 32))
	done := doneOutcome(t, st, 1)
	line := func(id string) string { return "continues: " + id + "\n" }
	if code, a, body := checkWorkOrder(t, owner, srv, workOrderText(line(mine.ID))); code != 200 || len(a.Problems) != 0 || a.Order == nil {
		t.Fatalf("running: %d %s", code, body)
	}
	_, _, notMine := checkWorkOrder(t, owner, srv, workOrderText(line(theirs.ID)))
	_, _, missing := checkWorkOrder(t, owner, srv, workOrderText(line(strings.Repeat("9", 32))))
	_, _, finished := checkWorkOrder(t, owner, srv, workOrderText(line(done.ID)))
	if !strings.Contains(notMine, "No running outcome of yours has that id.") || notMine != missing || notMine != finished || strings.Contains(notMine, `"line":1`) {
		t.Fatalf("not identical:\n%s\n%s\n%s", notMine, missing, finished)
	}
}

func TestWorkOrderRoutesRefuseReadOnlyAndSignedOutAndOversize(t *testing.T) {
	st, srv, owner := templateServer(t)
	password, _ := auth.HashPassword("long-enough-password")
	user, err := st.CreateUser("viewer@example.test", "viewer", "viewer", password)
	if err != nil {
		t.Fatal(err)
	}
	viewer := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, viewer, srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+user.Email+`","password":"long-enough-password"}`); code != 200 {
		t.Fatal(code)
	}
	o := doneOutcome(t, st, 1)
	for _, route := range [][2]string{{"POST", "/api/v2/gaffer/work-orders/check"}, {"GET", "/api/v2/gaffer/outcomes/" + o.ID + "/work-report"}} {
		body := ""
		if route[0] == "POST" {
			body = `{"text":""}`
		}
		if code, _ := gafferClient(t, &http.Client{}, srv, route[0], route[1], body); code != 401 {
			t.Fatalf("signed out %v: %d", route, code)
		}
		if code, _ := gafferClient(t, viewer, srv, route[0], route[1], body); code != 403 {
			t.Fatalf("read-only %v: %d", route, code)
		}
	}
	if code, _, _ := checkWorkOrder(t, owner, srv, strings.Repeat("x", 70<<10)); code != 413 {
		t.Fatalf("oversize: %d", code)
	}
}

func TestWorkReportRouteIsTheOwnersAndPlainText(t *testing.T) {
	st, srv, owner := templateServer(t)
	password, _ := auth.HashPassword("long-enough-password")
	user, err := st.CreateUser("member@example.test", "member", "member", password)
	if err != nil {
		t.Fatal(err)
	}
	member := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, member, srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+user.Email+`","password":"long-enough-password"}`); code != 200 {
		t.Fatal(code)
	}
	o := doneOutcome(t, st, 1)
	path := "/api/v2/gaffer/outcomes/" + o.ID + "/work-report"
	if code, body := gafferClient(t, owner, srv, "GET", path, ""); code != 200 || !strings.HasPrefix(body, "---\nreet: work-report/1\noutcome: "+o.ID) {
		t.Fatalf("owner: %d %s", code, body)
	}
	if code, body := gafferClient(t, member, srv, "GET", path, ""); code != 404 || strings.Contains(body, "Ship CSV") {
		t.Fatalf("other account: %d %s", code, body)
	}
}
