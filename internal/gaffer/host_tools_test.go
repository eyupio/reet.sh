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
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// fakeToolHost stands in for the controller's tools.
type fakeToolHost struct {
	mu    sync.Mutex
	tools []HostTool
	calls []string
	// text is, per call, whether the result may carry session text to the provider.
	text []bool
	fail map[string]string
}

func (f *fakeToolHost) GafferTools(int64) []HostTool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]HostTool(nil), f.tools...)
}
func (f *fakeToolHost) CallGafferTool(ctx context.Context, _ int64, name string, args json.RawMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name+" "+string(args))
	f.text = append(f.text, SessionTextAllowed(ctx))
	if why := f.fail[name]; why != "" {
		return "", errors.New(why)
	}
	return `{"data":"RESULT_OF_` + name + `"}`, nil
}
func (f *fakeToolHost) made() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// toolProvider answers each request from script: a tool call ("name args") or
// plain text. It records the tool names each request was offered and its body.
type toolProvider struct {
	mu      sync.Mutex
	script  []string
	offered [][]string
	bodies  []string
}

func (p *toolProvider) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body struct {
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(raw, &body)
	var names []string
	for _, tool := range body.Tools {
		names = append(names, tool.Function.Name)
	}
	p.mu.Lock()
	p.offered = append(p.offered, names)
	p.bodies = append(p.bodies, string(raw))
	step := "Nothing more."
	if len(p.script) > 0 {
		step, p.script = p.script[0], p.script[1:]
	}
	n := len(p.offered)
	p.mu.Unlock()
	if name, args, ok := strings.Cut(step, " "); ok && strings.HasPrefix(name, "reet_") || strings.HasPrefix(step, "reet_") {
		if !ok {
			name, args = step, "{}"
		}
		call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("c%d", n), "type": "function", "function": map[string]any{"name": name, "arguments": args}}}}, "finish_reason": "tool_calls"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", call)
		return
	}
	text, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": step}, "finish_reason": "stop"}}})
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", text)
}

func hostFixture(t *testing.T, tools []HostTool, script ...string) (*Service, *store.Store, Principal, ConversationView, *fakeToolHost, *toolProvider, *auditLog) {
	t.Helper()
	s, st, p := settingsFixture(t)
	host := &fakeToolHost{tools: tools, fail: map[string]string{}}
	s.SetToolHost(host)
	log := recordAudit(s)
	provider := &toolProvider{script: script}
	fake := httptest.NewServer(http.HandlerFunc(provider.serve))
	t.Cleanup(fake.Close)
	return s, st, p, conversationFixture(t, s, p, fake.URL), host, provider, log
}

func ask(t *testing.T, s *Service, p Principal, v ConversationView, id, text string) store.GafferTurn {
	t.Helper()
	c, err := s.store.GafferConversationFor(p.UserID, v.Conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartTurn(p.UserID, c.ID, TurnInput{ID: strings.Repeat(id, 32), Revision: c.Revision, Text: text}); err != nil {
		t.Fatal(err)
	}
	return waitTurn(t, s, p.UserID, c.ID, strings.Repeat(id, 32))
}

var (
	readTool  = HostTool{Name: "reet_list_factory", Title: "List Factory outcomes", Description: "Read the Factory.", Parameters: json.RawMessage(`{"type":"object","properties":{}}`), ReadOnly: true, Mode: "allow"}
	writeTool = HostTool{Name: "reet_stop_outcome", Title: "Stop an outcome", Description: "Stop it.", Parameters: json.RawMessage(`{"type":"object","properties":{"outcome_id":{"type":"string"}}}`), Mode: "allow"}
)

// A tool the person set to Allow runs in their chat turn, its result goes
// back to the model, the turn keeps what was run, and it is audited. Tools
// that are off are never offered, and a later turn is told what was run.
func TestGafferUsesAnAllowedToolAndReportsItsResult(t *testing.T) {
	off := HostTool{Name: "reet_delete_list", Mode: "off", Parameters: json.RawMessage(`{"type":"object"}`)}
	s, _, p, v, host, provider, log := hostFixture(t, []HostTool{readTool, writeTool, off},
		"reet_list_factory", `reet_stop_outcome {"outcome_id":"abc"}`, "Two outcomes; I stopped abc.", "As I said.")
	turn := ask(t, s, p, v, "a", "What is in the Factory? Stop abc.")
	if turn.State != "completed" || !strings.Contains(turn.Answer, "I stopped abc") {
		t.Fatalf("turn: %+v", turn)
	}
	if got := host.made(); len(got) != 2 || got[0] != "reet_list_factory {}" || got[1] != `reet_stop_outcome {"outcome_id":"abc"}` {
		t.Fatalf("calls made: %v", got)
	}
	if len(turn.Tools) != 2 || turn.Tools[0].State != "ran" || turn.Tools[0].Mode != "allow" || !strings.Contains(turn.Tools[0].Result, "RESULT_OF_reet_list_factory") || turn.Tools[1].Args != `{"outcome_id":"abc"}` {
		t.Fatalf("kept on the turn: %+v", turn.Tools)
	}
	provider.mu.Lock()
	first, second := strings.Join(provider.offered[0], ","), provider.bodies[1]
	provider.mu.Unlock()
	if first != "reet_list_factory,reet_stop_outcome" || !strings.Contains(second, "RESULT_OF_reet_list_factory") {
		t.Fatalf("offered %q; second request %s", first, second)
	}
	log.wait(t, "gaffer-tool-call", "gaffer-tool-call")
	if row := log.all()[1]; row.detail["tool"] != "reet_stop_outcome" || row.detail["mode"] != "allow" || row.detail["ok"] != true {
		t.Fatalf("audit: %+v", row)
	}
	// The next turn's history says what the tools did.
	ask(t, s, p, v, "b", "And now?")
	provider.mu.Lock()
	later := provider.bodies[len(provider.bodies)-1]
	provider.mu.Unlock()
	if !strings.Contains(later, "[reet tools in this turn]") || !strings.Contains(later, "reet_stop_outcome") {
		t.Fatalf("history did not carry the tools: %s", later)
	}
}

// A tool set to Ask me is not run. It waits on the turn; approving runs
// exactly the saved call, once; a second approval and an approval after the
// tool was switched off are refused; declining runs nothing.
func TestAToolSetToAskWaitsForThePersonAndRunsOnce(t *testing.T) {
	asking := writeTool
	asking.Mode = "ask"
	s, st, p, v, host, provider, log := hostFixture(t, []HostTool{asking},
		`reet_stop_outcome {"outcome_id":"abc"}`, "I’ve asked to stop abc; it waits for you.",
		`reet_stop_outcome {"outcome_id":"def"}`, "Asked again.",
		`reet_stop_outcome {"outcome_id":"ghi"}`, "And again.")
	turn := ask(t, s, p, v, "a", "Stop abc.")
	if turn.State != "completed" || len(turn.Tools) != 1 || turn.Tools[0].State != "pending" || len(host.made()) != 0 {
		t.Fatalf("asked: %+v made=%v", turn, host.made())
	}
	provider.mu.Lock()
	report, after := provider.bodies[1], provider.offered[1]
	provider.mu.Unlock()
	if !strings.Contains(report, "was NOT run") || len(after) != 0 {
		t.Fatalf("the model was not told it waits, or kept its tools: %v %s", after, report)
	}
	c := v.Conversation.ID
	if _, err := s.AnswerTool(context.Background(), p.UserID, c, turn.ID, "nope", true); err == nil {
		t.Fatal("an unknown request was approved")
	}
	done, err := s.AnswerTool(context.Background(), p.UserID, c, turn.ID, turn.Tools[0].ID, true)
	if err != nil || done.Tools[0].State != "ran" || !strings.Contains(done.Tools[0].Result, "RESULT_OF_reet_stop_outcome") {
		t.Fatalf("approved: %+v %v", done.Tools, err)
	}
	if got := host.made(); len(got) != 1 || got[0] != `reet_stop_outcome {"outcome_id":"abc"}` {
		t.Fatalf("the saved call was not the one run: %v", got)
	}
	if _, err = s.AnswerTool(context.Background(), p.UserID, c, turn.ID, turn.Tools[0].ID, true); err == nil || len(host.made()) != 1 {
		t.Fatalf("a second approval ran it again: %v %v", err, host.made())
	}
	// Declined: nothing runs.
	second := ask(t, s, p, v, "b", "Stop def.")
	declined, err := s.AnswerTool(context.Background(), p.UserID, c, second.ID, second.Tools[0].ID, false)
	if err != nil || declined.Tools[0].State != "declined" || len(host.made()) != 1 {
		t.Fatalf("declined: %+v %v %v", declined.Tools, err, host.made())
	}
	// Switched off before the answer: refused, and still waiting.
	third := ask(t, s, p, v, "c", "Stop ghi.")
	host.mu.Lock()
	host.tools = nil
	host.mu.Unlock()
	if _, err = s.AnswerTool(context.Background(), p.UserID, c, third.ID, third.Tools[0].ID, true); err == nil || len(host.made()) != 1 {
		t.Fatalf("a tool that is off was run: %v", err)
	}
	if kept, _ := st.GafferTurnFor(p.UserID, c, third.ID); kept.Tools[0].State != "pending" {
		t.Fatalf("refused approval changed the request: %+v", kept.Tools)
	}
	log.wait(t, "gaffer-tool-proposed", "gaffer-tool-call", "gaffer-tool-proposed", "gaffer-tool-declined", "gaffer-tool-proposed")
}

// A failing tool is reported to the model as failed, not as done, and no
// more than six tools run in one turn.
func TestAFailedToolIsReportedAndATurnStopsAtSixTools(t *testing.T) {
	script := []string{"reet_list_factory", "reet_list_factory", "reet_list_factory", "reet_list_factory", "reet_list_factory", "reet_list_factory", "Six looks."}
	s, _, p, v, host, provider, _ := hostFixture(t, []HostTool{readTool}, script...)
	host.fail["reet_list_factory"] = "the Factory could not be read"
	turn := ask(t, s, p, v, "a", "Look six times.")
	if turn.State != "completed" || len(turn.Tools) != 6 || turn.Tools[0].State != "failed" {
		t.Fatalf("turn: state=%s tools=%d %s", turn.State, len(turn.Tools), turn.Error)
	}
	provider.mu.Lock()
	second, last := provider.bodies[1], provider.offered[len(provider.offered)-1]
	provider.mu.Unlock()
	if !strings.Contains(second, "The tool failed") || len(last) != 0 {
		t.Fatalf("failure not reported, or tools still offered after six: %v", last)
	}
}

// In a session's chat the model also reads the working model's text, so a
// tool that changes something asks first even where the person chose Allow.
// A goal review is offered no reet tool at all.
func TestASessionChatAsksBeforeAChangeAndAGoalReviewHasNoReetTools(t *testing.T) {
	s, _, _ := settingsFixture(t)
	host := &fakeToolHost{tools: []HostTool{readTool, writeTool}}
	s.SetToolHost(host)
	general := s.hostTools(&chatJob{uid: 1}, store.GafferConversation{})
	session := s.hostTools(&chatJob{uid: 1}, store.GafferConversation{SessionID: "abc"})
	if general["reet_stop_outcome"].Mode != "allow" || session["reet_stop_outcome"].Mode != "ask" || session["reet_list_factory"].Mode != "allow" {
		t.Fatalf("general %+v session %+v", general, session)
	}
	if review := s.hostTools(&chatJob{uid: 1, goalID: "g"}, store.GafferConversation{SessionID: "abc"}); len(review) != 0 {
		t.Fatalf("a goal review was offered reet tools: %+v", review)
	}
}

// A request left waiting for more than a day is not run on approval: what it
// would act on may have moved on. It is closed, and the person is told.
func TestAToolRequestLeftTooLongIsNotRunOnApproval(t *testing.T) {
	asking := writeTool
	asking.Mode = "ask"
	s, st, p, v, host, _, _ := hostFixture(t, []HostTool{asking}, `reet_stop_outcome {"outcome_id":"abc"}`, "Asked.")
	turn := ask(t, s, p, v, "a", "Stop abc.")
	life := pendingToolLife
	pendingToolLife = 0
	t.Cleanup(func() { pendingToolLife = life })
	time.Sleep(5 * time.Millisecond)
	c := v.Conversation.ID
	if _, err := s.AnswerTool(context.Background(), p.UserID, c, turn.ID, turn.Tools[0].ID, true); err == nil || !strings.Contains(err.Error(), "more than a day") || len(host.made()) != 0 {
		t.Fatalf("an old request was run: %v %v", err, host.made())
	}
	if kept, _ := st.GafferTurnFor(p.UserID, c, turn.ID); kept.Tools[0].State != "declined" || !strings.Contains(kept.Tools[0].Result, "more than a day") {
		t.Fatalf("the old request stays open: %+v", kept.Tools)
	}
}

// What a tool returns goes to the chat's provider. A provider that is not
// allowed session text is not handed it through a tool.
func TestAToolCallSaysWhetherItsResultMayCarrySessionText(t *testing.T) {
	s, st, p, v, host, _, _ := hostFixture(t, []HostTool{readTool}, "reet_list_factory", "Read.", "reet_list_factory", "Read again.")
	ask(t, s, p, v, "a", "What is on the floor?")
	row, err := st.GafferProviderFor(p.UserID, v.Conversation.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	first := append([]bool(nil), host.text...)
	host.mu.Unlock()
	if len(first) != 1 || first[0] != (row.AllowSessions && row.AllowCode) {
		t.Fatalf("session text for this provider: %v, provider allows %v %v", first, row.AllowSessions, row.AllowCode)
	}
	if !SessionTextAllowed(context.Background()) {
		t.Fatal("a call that is not Gaffer's is not restricted here")
	}
}
