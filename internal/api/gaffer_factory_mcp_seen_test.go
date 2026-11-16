package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// factoryMCPFixture is a controller with remote sessions on and one owner,
// whose Factory tools are called as a connection with management scope.
func factoryMCPFixture(t *testing.T) (*server, store.User, mcpAccess) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithRemoteTerminals(true), WithGafferProviders(seal))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	user, ok, err := st.UserByEmail("owner@example.test")
	if err != nil || !ok {
		t.Fatalf("owner: %v %v", ok, err)
	}
	return h.s, user, mcpAccess{User: user, Connection: store.MCPConnection{ID: 2, Name: "manager", Scope: "write"}}
}

// factoryRecord saves an outcome with one pipeline in the given state.
func factoryRecord(t *testing.T, s *server, uid int64, id, state string, change func(*store.GafferOutcomeSession)) store.GafferOutcome {
	t.Helper()
	o, err := s.st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: strings.Repeat(id, 32), Objective: "Ship it.", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it.", RequestID: strings.Repeat("8", 32), Stages: []store.GafferOutcomeStage{
		{Kind: "code", Tool: "opencode", State: "done"}, {Kind: "merge", Tool: "opencode", State: "pending"},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	o, err = s.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State = "settled", state
		if change != nil {
			change(&o.Sessions[0])
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// An approval over MCP is of the pipeline as it was read. Work that was sent
// back and done again since is a different thing to approve: the earlier
// approval, or one Gaffer asked for a while ago, does not carry over to it.
func TestFactoryMCPMergeApprovalIsOfThePipelineAsItWasRead(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	ctx := context.Background()
	o := factoryRecord(t, s, user.ID, "c", "ready_to_merge", func(p *store.GafferOutcomeSession) { p.Stage, p.UpdatedAt = 1, 5000 })
	args := func(seen any) map[string]any {
		out := map[string]any{"outcome_id": o.ID, "pipeline": "a", "confirm": true}
		if seen != nil {
			out["seen_updated_at"] = seen
		}
		return out
	}
	for name, seen := range map[string]any{"no read": nil, "an older read": float64(4000)} {
		if _, err := s.executeMCPFactory(ctx, write, "reet_approve_pipeline_merge", args(seen)); err == nil || !strings.Contains(err.Error(), "Nothing was approved") {
			t.Fatalf("approval with %s: %v", name, err)
		}
		if now, err := s.st.GafferOutcomeFor(user.ID, o.ID); err != nil || now.Sessions[0].MergeApproved || now.Sessions[0].State != "ready_to_merge" {
			t.Fatalf("approval with %s changed the pipeline: %+v %v", name, now.Sessions[0], err)
		}
	}
	out, err := s.executeMCPFactory(ctx, write, "reet_approve_pipeline_merge", args(float64(5000)))
	if err != nil || out["applied"] != true {
		t.Fatalf("approval of what was read: %v %v", out, err)
	}
	if now, err := s.st.GafferOutcomeFor(user.ID, o.ID); err != nil || !now.Sessions[0].MergeApproved {
		t.Fatalf("approved: %+v %v", now.Sessions[0], err)
	}
}

// A control that changed nothing says so: stopping an outcome that has
// already finished is not reported as applied.
func TestFactoryMCPStopOfAFinishedOutcomeIsNotReportedAsApplied(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	o := factoryRecord(t, s, user.ID, "d", "done", nil)
	out, err := s.executeMCPFactory(context.Background(), write, "reet_stop_outcome", map[string]any{"outcome_id": o.ID, "confirm": true})
	if err != nil || out["applied"] != false {
		t.Fatalf("stop of a finished outcome: %v %v", out, err)
	}
	if now, err := s.st.GafferOutcomeFor(user.ID, o.ID); err != nil || now.Revision != o.Revision {
		t.Fatalf("the finished record changed: %v", err)
	}
}

// An outcome is started over MCP only on a machine known to be able to run
// the agent as a chat. A machine that has not said what it can run is not
// taken to be able: the outcome is refused before it is saved.
func TestFactoryMCPStartIsRefusedWhenTheMachineHasNotSaidWhatItRuns(t *testing.T) {
	t.Parallel()
	s, user, write := factoryMCPFixture(t)
	agent, _, err := s.st.EnrollSyncAgent(user.ID, "dark-machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.st.CreateFolderProjectFor(user.ID, store.FolderProject{Host: agent.ID, Dir: "/work/billing", Name: "billing", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.executeMCPFactory(context.Background(), write, "reet_start_outcome", map[string]any{
		"objective": "Ship CSV export.", "project_id": project.ID, "agent": "opencode", "confirm": true,
		"pipelines": []any{map[string]any{"title": "Export", "prompt": "Add CSV export. Do not merge."}},
	})
	if err == nil || !strings.Contains(err.Error(), "Nothing was started") {
		t.Fatalf("start on a machine that reported nothing: %v", err)
	}
	if rows, err := s.st.GafferOutcomesFor(user.ID); err != nil || len(rows) != 0 {
		t.Fatalf("an outcome was saved: %d %v", len(rows), err)
	}
}

// The tools whose answers carry handover notes and reports are marked, so a
// change Gaffer asks for after reading them waits for the person.
func TestGafferToolsThatReturnSessionTextAreMarked(t *testing.T) {
	t.Parallel()
	s, user, _ := factoryMCPFixture(t)
	modes := map[string]string{}
	for _, tool := range mcpToolDefinitions {
		if gafferToolOffered(tool.Name) {
			modes[tool.Name] = "allow"
		}
	}
	if err := s.st.SetGafferToolModesFor(user.ID, modes); err != nil {
		t.Fatal(err)
	}
	marked := map[string]bool{}
	for _, tool := range (gafferToolHost{s}).GafferTools(user.ID) {
		marked[tool.Name] = tool.SessionText
	}
	if !marked["reet_get_factory_outcome"] || !marked["reet_report_outcome"] || !marked["reet_retry_pipeline"] {
		t.Fatalf("tools that return handovers or reports are not marked: %v", marked)
	}
	if marked["reet_list_factory"] || marked["reet_list_factory_projects"] || marked["reet_list_skills"] {
		t.Fatalf("tools that return no session text are marked: %v", marked)
	}
}
