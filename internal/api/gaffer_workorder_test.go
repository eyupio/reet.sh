package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
)

// workOrderExample is the skill's own worked example
// (internal/gaffer/testdata/workorder_example.md), copied here so a change to
// either is caught by both packages' tests.
const workOrderExample = `---
reet: work-order/1
objective: Add CSV export to the reports page
project: reports-api
crew: heavy=claude/opus/high, standard=codex
---

## Pipeline: Export endpoint

stages: plan, code, review, test, pr
tiers: code=heavy, review=standard

Add GET /reports/{id}/export.csv … (the brief: self-contained prose, any Markdown,
up to the first ### heading)

### Part: code / Streaming writer
tier: heavy

Write rows as they are read, never the whole file in memory.

### Part: code / Endpoint and auth

Wire the handler and reuse the report's read permission.

### Criteria

- An export of 10,000 rows finishes in under 5 seconds
- A reader of another team's report is refused
`

// workOrderCheckBody JSON-encodes text as the check route's body.
func workOrderCheckBody(text string) string {
	raw, _ := json.Marshal(map[string]string{"text": text})
	return string(raw)
}

type workOrderCheckOut struct {
	Order     *gaffer.WorkOrder         `json:"order"`
	Problems  []gaffer.WorkOrderProblem `json:"problems"`
	ProjectID string                    `json:"projectId"`
	PathwayID string                    `json:"pathwayId"`
	Notes     []string                  `json:"notes"`
}

func checkWorkOrder(t *testing.T, client *http.Client, srv *httptest.Server, text string) (int, workOrderCheckOut) {
	t.Helper()
	code, got := gafferClient(t, client, srv, "POST", "/api/v2/gaffer/work-orders/check", workOrderCheckBody(text))
	var out workOrderCheckOut
	if code == 200 {
		if err := json.Unmarshal([]byte(got), &out); err != nil {
			t.Fatalf("decode %s: %v", got, err)
		}
	}
	return code, out
}

// workOrderStart is an owner account behind a server with one Factory
// project, named to match the skill's example's "project: reports-api" hint
// in any letter case.
func workOrderStart(t *testing.T) (*store.Store, *httptest.Server, *http.Client, store.FolderProject) {
	t.Helper()
	st, srv, owner := templateServer(t)
	agent, _, err := st.EnrollSyncAgent(1, "machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateFolderProjectFor(1, store.FolderProject{Host: agent.ID, Dir: "/work/reports-api", Name: "Reports-API", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	return st, srv, owner, project
}

// A clean work order parses with no problems, one pipeline, and its project
// hint matched case-insensitively against the account's own project.
func TestWorkOrderCheckCleanExampleMatchesTheProject(t *testing.T) {
	_, srv, owner, project := workOrderStart(t)
	code, out := checkWorkOrder(t, owner, srv, workOrderExample)
	if code != 200 {
		t.Fatalf("check: %d", code)
	}
	if len(out.Problems) != 0 || out.Order == nil || len(out.Order.Sessions) != 1 {
		t.Fatalf("clean example: %+v", out)
	}
	if out.ProjectID != project.ID || len(out.Notes) != 0 {
		t.Fatalf("project match: %+v", out)
	}
}

// A project hint that matches none of the account's projects is a note, not
// a problem, and leaves projectId empty: the person still chooses.
func TestWorkOrderCheckUnknownProjectIsANote(t *testing.T) {
	_, srv, owner := templateServer(t)
	code, out := checkWorkOrder(t, owner, srv, workOrderExample)
	if code != 200 {
		t.Fatalf("check: %d", code)
	}
	if len(out.Problems) != 0 || out.ProjectID != "" || len(out.Notes) != 1 {
		t.Fatalf("unknown project: %+v", out)
	}
}

func readOnlyClient(t *testing.T, st *store.Store, srv *httptest.Server) *http.Client {
	t.Helper()
	password, err := auth.HashPassword("long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.CreateUser("viewer@example.test", "viewer", "viewer", password)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, client, srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+user.Email+`","password":"long-enough-password"}`); code != 200 {
		t.Fatal(code)
	}
	return client
}

// A read-only account is refused the check route, the same way it is refused
// giving Gaffer an outcome.
func TestWorkOrderCheckRefusesAReadOnlyAccount(t *testing.T) {
	st, srv, _ := templateServer(t)
	viewer := readOnlyClient(t, st, srv)
	code, _ := gafferClient(t, viewer, srv, "POST", "/api/v2/gaffer/work-orders/check", workOrderCheckBody(workOrderExample))
	if code != 403 {
		t.Fatalf("read-only check: %d", code)
	}
}

// A body too large for the decode cap to even reach the parser never starts
// a dry run; one well under that cap but over the work order's own byte
// limit is parsed far enough to report that limit as one problem, with no
// order to check.
func TestWorkOrderCheckOversizedBodyIsRefusedOrOneProblem(t *testing.T) {
	_, srv, owner := templateServer(t)
	huge := strings.Repeat("x", gaffer.MaxWorkOrderBytes+500)
	code, out := checkWorkOrder(t, owner, srv, huge)
	switch {
	case code == 200:
		if out.Order != nil || len(out.Problems) != 1 {
			t.Fatalf("oversized within decode cap: %+v", out)
		}
	case code == 400 || code == 413:
		// Refused before parsing, as decodeStrictJSON does for any oversized body.
	default:
		t.Fatalf("oversized body: %d", code)
	}
	beyondDecodeCap := strings.Repeat("x", gaffer.MaxWorkOrderBytes+2000)
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/work-orders/check", workOrderCheckBody(beyondDecodeCap)); code != 400 && code != 413 {
		t.Fatalf("body beyond the decode cap: %d", code)
	}
}

func continuesWorkOrder(id string) string {
	return "---\nreet: work-order/1\ncontinues: " + id + "\n---\n\n## Pipeline: Follow-up\n\nDo more work.\n"
}

// continuesTestOutcome stores a minimal, running outcome for uid directly,
// bypassing the launch path, as doneOutcome does for other tests.
func continuesTestOutcome(t *testing.T, st *store.Store, uid int64, id string) store.GafferOutcome {
	t.Helper()
	o, err := st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: id, Objective: "Ship it.", AgentID: "machine-1", ProjectID: "project-1", Dir: "/work", Tool: "opencode", MaxReviews: 8, Hours: 24,
		Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Pipeline", Prompt: "Do it.", RequestID: strings.Repeat("c", 32)}}})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// continues naming a running, unarchived outcome of the caller is not a
// problem.
func TestWorkOrderCheckContinuesOwnRunningOutcomeHasNoProblem(t *testing.T) {
	st, srv, owner := templateServer(t)
	o := continuesTestOutcome(t, st, 1, strings.Repeat("a", 32))
	code, out := checkWorkOrder(t, owner, srv, continuesWorkOrder(o.ID))
	if code != 200 || out.Order == nil || len(out.Problems) != 0 {
		t.Fatalf("continues own outcome: %d %+v", code, out)
	}
}

// continues naming another account's outcome is a problem at line 0, and the
// response is exactly the same as for an id that does not exist at all: the
// caller is never told which it was.
func TestWorkOrderCheckContinuesAnotherAccountsOutcomeLooksLikeNoSuchID(t *testing.T) {
	st, srv, owner := templateServer(t)
	password, err := auth.HashPassword("long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser("other@example.test", "Other", "member", password)
	if err != nil {
		t.Fatal(err)
	}
	theirs := continuesTestOutcome(t, st, other.ID, strings.Repeat("b", 32))
	_, gotTheirs := checkWorkOrder(t, owner, srv, continuesWorkOrder(theirs.ID))
	_, gotMissing := checkWorkOrder(t, owner, srv, continuesWorkOrder(strings.Repeat("z", 32)))
	if len(gotTheirs.Problems) != 1 || len(gotMissing.Problems) != 1 {
		t.Fatalf("continues problems: theirs=%+v missing=%+v", gotTheirs, gotMissing)
	}
	if gotTheirs.Problems[0] != gotMissing.Problems[0] {
		t.Fatalf("continues leaked which id exists: theirs=%+v missing=%+v", gotTheirs.Problems, gotMissing.Problems)
	}
}

func pathwayWorkOrder(name string) string {
	return "---\nreet: work-order/1\nobjective: Tidy the logs.\npathway: " + name + "\n---\n\n## Pipeline: Logs\n\nTidy the logs.\n"
}

// A named pathway that is switched on sets pathwayId and is not a problem.
func TestWorkOrderCheckEnabledPathwaySetsPathwayID(t *testing.T) {
	st, srv, owner, _ := workOrderStart(t)
	p := gafferSavePathway(t, st, "Careful", true)
	code, out := checkWorkOrder(t, owner, srv, pathwayWorkOrder(p.Name))
	if code != 200 || len(out.Problems) != 0 || out.PathwayID != p.ID {
		t.Fatalf("enabled pathway: %d %+v", code, out)
	}
}

// A named pathway that is switched off is a problem, not a note, per the
// design: a pathway that cannot run is not something the person "chooses"
// around the way an unmatched project is.
func TestWorkOrderCheckDisabledPathwayIsAProblem(t *testing.T) {
	st, srv, owner, _ := workOrderStart(t)
	p := gafferSavePathway(t, st, "Quiet", false)
	code, out := checkWorkOrder(t, owner, srv, pathwayWorkOrder(p.Name))
	if code != 200 || len(out.Problems) != 1 || out.PathwayID != "" {
		t.Fatalf("disabled pathway: %d %+v", code, out)
	}
}

// A pathway name matching nothing at all is treated the same way an
// unmatched project is: a note, not a problem, with pathwayId left empty.
func TestWorkOrderCheckUnknownPathwayIsANote(t *testing.T) {
	_, srv, owner, _ := workOrderStart(t)
	code, out := checkWorkOrder(t, owner, srv, pathwayWorkOrder("No Such Pathway"))
	if code != 200 || len(out.Problems) != 0 || out.PathwayID != "" || len(out.Notes) != 1 {
		t.Fatalf("unknown pathway: %d %+v", code, out)
	}
}

// gafferSavePathway saves a pathway directly through the store, the way
// pathwayStart does, without needing a configured provider.
func gafferSavePathway(t *testing.T, st *store.Store, name string, enabled bool) store.GafferPathway {
	t.Helper()
	p, err := st.SaveGafferPathwayFor(1, store.GafferPathway{Name: name, Enabled: enabled, Stages: []store.GafferPathwayStage{{Kind: "code"}, {Kind: "pr"}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// GET .../work-report returns the plain-text report for one of the caller's
// own outcomes.
func TestWorkOrderReportReturnsPlainTextForTheCallersOutcome(t *testing.T) {
	st, srv, owner := templateServer(t)
	o := continuesTestOutcome(t, st, 1, strings.Repeat("d", 32))
	code, got := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/outcomes/"+o.ID+"/work-report", "")
	if code != 200 {
		t.Fatalf("work report: %d %s", code, got)
	}
	if !strings.Contains(got, "reet: work-report/1") || !strings.Contains(got, "outcome: "+o.ID) {
		t.Fatalf("work report body: %s", got)
	}
}

// The report route for another account's outcome is not found, the same as
// any other outcome route.
func TestWorkOrderReportForAnotherAccountsOutcomeIs404(t *testing.T) {
	st, srv, owner := templateServer(t)
	password, err := auth.HashPassword("long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser("other2@example.test", "Other", "member", password)
	if err != nil {
		t.Fatal(err)
	}
	theirs := continuesTestOutcome(t, st, other.ID, strings.Repeat("e", 32))
	code, _ := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/outcomes/"+theirs.ID+"/work-report", "")
	if code != 404 {
		t.Fatalf("other account's report: %d", code)
	}
}
