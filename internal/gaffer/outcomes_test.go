package gaffer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// planServer answers every chat request with one propose_sessions call whose
// arguments are args, and keeps what the provider was sent.
func planServer(t *testing.T, reply func() string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()
		fmt.Fprint(w, reply())
	}))
	t.Cleanup(server.Close)
	return server, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), bodies...) }
}

func planCall(name, args string) string {
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "plan_one", "type": "function", "function": map[string]any{"name": name, "arguments": args}}}}, "finish_reason": "tool_calls"}}})
	return "data: " + string(call) + "\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":40,\"completion_tokens\":60}}\n\ndata: [DONE]\n\n"
}

func planProvider(t *testing.T, s *Service, p Principal, url string, enabled bool) store.GafferProvider {
	t.Helper()
	key := "synthetic-key"
	got, err := s.Save(p, Input{Name: "Planner", Kind: "openai_compatible", BaseURL: url, Model: "plan-model", Enabled: enabled, LocalOnly: true, APIKey: &key})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPlanOutcomeReturnsAnEditablePlanFromTheTypedObjectiveOnly(t *testing.T) {
	s, st, p := settingsFixture(t)
	args := `{"summary":"Two independent halves.","sessions":[{"title":"Export API","prompt":"Add the CSV export endpoint with tests. Raise a pull request. Do not merge."},{"title":"   ","prompt":"Add the export button to the billing page.\nRaise a pull request. Do not merge."}]}`
	server, sent := planServer(t, func() string { return planCall("propose_sessions", args) })
	row := planProvider(t, s, p, server.URL, true)
	plan, err := s.PlanOutcome(context.Background(), p, row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "  Ship CSV export for billing.  ", Project: "billing", Sessions: 3})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary != "Two independent halves." || len(plan.Sessions) != 2 || plan.Model != "plan-model" || !plan.UsageReported || plan.InputTokens != 40 || plan.OutputTokens != 60 {
		t.Fatalf("plan: %+v", plan)
	}
	if plan.Sessions[0].Title != "Export API" || plan.Sessions[1].Title == "" || strings.TrimSpace(plan.Sessions[1].Title) != plan.Sessions[1].Title {
		t.Fatalf("titles: %+v", plan.Sessions)
	}
	bodies := sent()
	if len(bodies) != 1 {
		t.Fatalf("provider requests: %d", len(bodies))
	}
	for _, want := range []string{"Ship CSV export for billing.", "Project: billing", "propose_sessions", "at most 3 sessions", "Do not merge."} {
		if !strings.Contains(bodies[0], want) {
			t.Fatalf("provider request lacks %q: %s", want, bodies[0])
		}
	}
	// Planning saves no conversation, goal, grant or provider permission.
	if list, err := st.GafferConversationsFor(p.UserID); err != nil || len(list) != 0 {
		t.Fatalf("conversations after planning: %v %v", list, err)
	}
	after, err := st.GafferProviderFor(p.UserID, row.ID)
	if err != nil || after.Revision != row.Revision || after.AllowSessions || after.AllowCode {
		t.Fatalf("provider changed by planning: %+v %v", after, err)
	}
}

func TestPlanOutcomeSendsThePersonsReplyAndPreviousProposal(t *testing.T) {
	s, _, p := settingsFixture(t)
	args := `{"summary":"One session, end to end.","sessions":[{"title":"Export, end to end","prompt":"Build the CSV export with tests. Raise a pull request. Do not merge."}]}`
	server, sent := planServer(t, func() string { return planCall("propose_sessions", args) })
	row := planProvider(t, s, p, server.URL, true)
	previous := &OutcomePlanPrevious{
		Summary:  "  Two independent halves.  ",
		Sessions: []OutcomePlanPreviousSession{{Title: "  Export API  ", Stages: []string{"code", "pr"}}, {Title: "Export button"}},
	}
	plan, err := s.PlanOutcome(context.Background(), p, row.ID, OutcomePlanInput{
		Revision: row.Revision, Objective: "Ship CSV export for billing.", Project: "billing", Sessions: 2,
		Reply: "  Not that: keep the export in one session and put its tests with it.  ", Previous: previous,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary != "One session, end to end." || len(plan.Sessions) != 1 {
		t.Fatalf("plan: %+v", plan)
	}
	bodies := sent()
	if len(bodies) != 1 {
		t.Fatalf("provider requests: %d", len(bodies))
	}
	for _, want := range []string{
		"Your previous proposal:", "Summary: Two independent halves.", "Session 1: Export API (stages: code, pr)", "Session 2: Export button",
		"The person replied:", "Not that: keep the export in one session and put its tests with it.", "Revise the proposal as a direct response",
	} {
		if !strings.Contains(bodies[0], want) {
			t.Fatalf("provider request lacks %q: %s", want, bodies[0])
		}
	}
}

func TestPlanOutcomeRefusesBadInputBeforeAnyProviderRequest(t *testing.T) {
	s, _, p := settingsFixture(t)
	server, sent := planServer(t, func() string {
		return planCall("propose_sessions", `{"summary":"x","sessions":[{"title":"a","prompt":"b"}]}`)
	})
	row := planProvider(t, s, p, server.URL, true)
	paused := planProvider2(t, s, p, server.URL)
	ok := OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Sessions: 2}
	cases := map[string]struct {
		id string
		in OutcomePlanInput
	}{
		"empty objective":   {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "  ", Sessions: 2}},
		"huge objective":    {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: strings.Repeat("a", MaxOutcomeObjectiveBytes+1), Sessions: 2}},
		"no sessions":       {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Sessions: 0}},
		"too many":          {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Sessions: MaxOutcomeSessions + 1}},
		"multiline project": {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Project: "a\nb", Sessions: 1}},
		"paused provider":   {paused.ID, OutcomePlanInput{Revision: paused.Revision, Objective: "Ship it.", Sessions: 1}},
		"huge reply":        {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Sessions: 2, Reply: strings.Repeat("r", MaxOutcomeReplyBytes+1)}},
		"control reply":     {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Sessions: 2, Reply: "Do it\x00now"}},
		"bad previous":      {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Sessions: 2, Reply: "Try again.", Previous: &OutcomePlanPrevious{Sessions: []OutcomePlanPreviousSession{{Title: "a\nb"}}}}},
		"unknown stage":     {row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Sessions: 2, Reply: "Try again.", Previous: &OutcomePlanPrevious{Sessions: []OutcomePlanPreviousSession{{Title: "Export", Stages: []string{"deploy"}}}}}},
	}
	for name, c := range cases {
		var bad *Invalid
		if _, err := s.PlanOutcome(context.Background(), p, c.id, c.in); !errors.As(err, &bad) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	stale := ok
	stale.Revision++
	if _, err := s.PlanOutcome(context.Background(), p, row.ID, stale); !errors.Is(err, store.ErrGafferConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	if _, err := s.PlanOutcome(context.Background(), Principal{UserID: p.UserID + 99, CanUseLocal: true}, row.ID, ok); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another account's provider: %v", err)
	}
	if n := len(sent()); n != 0 {
		t.Fatalf("refused requests reached the provider %d times", n)
	}
}

func planProvider2(t *testing.T, s *Service, p Principal, url string) store.GafferProvider {
	t.Helper()
	key := "synthetic-key"
	got, err := s.Save(p, Input{Name: "Paused planner", Kind: "openai_compatible", BaseURL: url, Model: "plan-model", Enabled: false, LocalOnly: true, APIKey: &key})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPlanOutcomeRejectsAnythingThatIsNotOneCompletePlan(t *testing.T) {
	long := strings.Repeat("p", MaxOutcomePromptBytes+1)
	replies := map[string]string{
		"prose only":       "data: {\"choices\":[{\"delta\":{\"content\":\"I would split it in two.\"}}]}\n\ndata: [DONE]\n\n",
		"another tool":     planCall("run_shell", `{}`),
		"more than asked":  planCall("propose_sessions", `{"summary":"s","sessions":[{"title":"a","prompt":"x"},{"title":"b","prompt":"y"},{"title":"c","prompt":"z"}]}`),
		"no sessions":      planCall("propose_sessions", `{"summary":"s","sessions":[]}`),
		"unknown field":    planCall("propose_sessions", `{"summary":"s","sessions":[{"title":"a","prompt":"x","yolo":true}]}`),
		"empty prompt":     planCall("propose_sessions", `{"summary":"s","sessions":[{"title":"a","prompt":"  "}]}`),
		"oversized prompt": planCall("propose_sessions", `{"summary":"s","sessions":[{"title":"a","prompt":"`+long+`"}]}`),
		"repeated title":   planCall("propose_sessions", `{"summary":"s","sessions":[{"title":"a","prompt":"x"},{"title":"a","prompt":"y"}]}`),
		"not json":         planCall("propose_sessions", `{"summary":`),
		"cut short":        "data: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n",
	}
	for name, reply := range replies {
		s, _, p := settingsFixture(t)
		server, _ := planServer(t, func() string { return reply })
		row := planProvider(t, s, p, server.URL, true)
		var probe *ProbeError
		plan, err := s.PlanOutcome(context.Background(), p, row.ID, OutcomePlanInput{Revision: row.Revision, Objective: "Ship it.", Sessions: 2})
		if !errors.As(err, &probe) || len(plan.Sessions) != 0 {
			t.Fatalf("%s: %+v %v", name, plan, err)
		}
	}
}
