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

	"github.com/eyupio/reet.sh/internal/elicore"
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

// A request nobody comes back to is closed by the controller's own look, not
// only when someone tries to approve it: after a day the chat stops showing
// it as waiting, and the close is recorded. A fresh request is left alone.
func TestAToolRequestLeftTooLongIsClosedWithoutAnyoneAnswering(t *testing.T) {
	asking := writeTool
	asking.Mode = "ask"
	s, st, p, v, host, _, _ := hostFixture(t, []HostTool{asking}, `reet_stop_outcome {"outcome_id":"abc"}`, "Asked.")
	log := recordAudit(s)
	turn := ask(t, s, p, v, "a", "Stop abc.")
	c := v.Conversation.ID
	s.expirePendingTools()
	if kept, _ := st.GafferTurnFor(p.UserID, c, turn.ID); kept.Tools[0].State != "pending" {
		t.Fatalf("a fresh request was closed: %+v", kept.Tools)
	}
	life := pendingToolLife
	pendingToolLife = 0
	t.Cleanup(func() { pendingToolLife = life })
	time.Sleep(5 * time.Millisecond)
	// The look is spaced out; a second one straight away does nothing.
	s.expirePendingTools()
	if kept, _ := st.GafferTurnFor(p.UserID, c, turn.ID); kept.Tools[0].State != "pending" {
		t.Fatalf("looked again within ten minutes: %+v", kept.Tools)
	}
	s.mu.Lock()
	s.toolsSwept = time.Time{}
	s.mu.Unlock()
	s.expirePendingTools()
	kept, _ := st.GafferTurnFor(p.UserID, c, turn.ID)
	if kept.Tools[0].State != "declined" || !strings.Contains(kept.Tools[0].Result, "more than a day") || len(host.made()) != 0 {
		t.Fatalf("the old request stays open: %+v", kept.Tools)
	}
	found := false
	for _, row := range log.all() {
		found = found || row.action == "gaffer-tool-declined" && row.detail["expired"] == true && row.detail["turn"] == turn.ID
	}
	if !found {
		t.Fatalf("the close was not recorded: %+v", log.all())
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

// A tool whose result carries words a working agent wrote (a handover note, a
// report) puts those words in front of the model. Whatever the model then asks
// to change in the same turn waits for the person, even on Allow: text from a
// working agent can propose a change but never make one.
func TestAChangeAfterReadingSessionTextAsksEvenOnAllow(t *testing.T) {
	reading := HostTool{Name: "reet_get_factory_outcome", Title: "Read one outcome", Description: "Read it.", Parameters: json.RawMessage(`{"type":"object","properties":{"outcome_id":{"type":"string"}}}`), ReadOnly: true, Mode: "allow", SessionText: true}
	s, _, p, v, host, _, _ := hostFixture(t, []HostTool{readTool, reading, writeTool},
		"reet_list_factory", `reet_get_factory_outcome {"outcome_id":"abc"}`, "reet_list_factory", `reet_stop_outcome {"outcome_id":"abc"}`, "I’ve asked to stop abc.")
	turn := ask(t, s, p, v, "a", "Read abc and stop it.")
	if turn.State != "completed" || len(turn.Tools) != 4 {
		t.Fatalf("turn: %+v", turn)
	}
	if got := host.made(); len(got) != 3 || strings.Contains(strings.Join(got, " "), "reet_stop_outcome") {
		t.Fatalf("a change ran after session text was read: %v", got)
	}
	if last := turn.Tools[3]; last.Name != "reet_stop_outcome" || last.State != "pending" || last.Mode != "ask" {
		t.Fatalf("the change should wait for the person: %+v", last)
	}
	if !turn.Tools[1].SessionText || turn.Tools[0].SessionText {
		t.Fatalf("which results carry session text: %+v", turn.Tools)
	}
	// Without session text in the turn, Allow still runs at once.
	s2, _, p2, v2, host2, _, _ := hostFixture(t, []HostTool{readTool, reading, writeTool}, "reet_list_factory", `reet_stop_outcome {"outcome_id":"abc"}`, "Stopped.")
	if again := ask(t, s2, p2, v2, "a", "Stop abc."); len(again.Tools) != 2 || again.Tools[1].State != "ran" || len(host2.made()) != 2 {
		t.Fatalf("allow without session text: %+v", again.Tools)
	}
}

// A later turn is told what earlier tools did. A result that carried session
// text is not repeated to a provider that may no longer read session text.
func TestStoredSessionTextIsNotReplayedToAProviderThatLostIt(t *testing.T) {
	tools := []store.GafferTurnTool{
		{Name: "reet_get_factory_outcome", Args: `{}`, State: "ran", Result: "HANDOVER_WORDS", SessionText: true},
		{Name: "reet_list_factory", Args: `{}`, State: "ran", Result: "STATES_ONLY"},
	}
	if with := toolHistory(tools, true); !strings.Contains(with, "HANDOVER_WORDS") || !strings.Contains(with, "STATES_ONLY") {
		t.Fatalf("allowed provider: %s", with)
	}
	without := toolHistory(tools, false)
	if strings.Contains(without, "HANDOVER_WORDS") || !strings.Contains(without, "STATES_ONLY") || !strings.Contains(without, "reet_get_factory_outcome") {
		t.Fatalf("provider without session text: %s", without)
	}
}

// A call approved and started whose result was never saved is not described as
// one still waiting: it may have run.
func TestAnApprovedCallWithNoRecordedResultMayHaveRun(t *testing.T) {
	got := toolHistory([]store.GafferTurnTool{{Name: "reet_stop_outcome", Args: `{}`, State: "approved"}, {Name: "reet_list_factory", Args: `{}`, State: "pending"}}, true)
	if !strings.Contains(got, "it may have run") || strings.Count(got, "it has not run") != 1 {
		t.Fatalf("history: %s", got)
	}
}

// Starting an outcome carries up to five briefs of 8,000 bytes: a call that
// size is a valid request, not a malformed one.
func TestAToolCallMayCarryAFullOutcome(t *testing.T) {
	big, _ := json.Marshal(map[string]any{"prompt": strings.Repeat("a", 48<<10)})
	if _, err := hostToolArgs(elicore.ToolCall{ID: "c1", Arguments: big}); err != nil {
		t.Fatalf("a full outcome was refused: %v", err)
	}
	huge, _ := json.Marshal(map[string]any{"prompt": strings.Repeat("a", 70<<10)})
	if _, err := hostToolArgs(elicore.ToolCall{ID: "c1", Arguments: huge}); err == nil {
		t.Fatal("an oversized call was accepted")
	}
	// The provider is given the room to write such a call, only where reet's tools are on offer.
	for _, tools := range [][]HostTool{{writeTool}, nil} {
		s, _, p, v, _, provider, _ := hostFixture(t, tools, "Nothing to do.")
		ask(t, s, p, v, "a", "Hello.")
		want := `"max_tokens":4096`
		if len(tools) > 0 {
			want = `"max_tokens":16384`
		}
		provider.mu.Lock()
		body := provider.bodies[0]
		provider.mu.Unlock()
		if !strings.Contains(body, want) {
			t.Fatalf("with %d tools the request lacks %s", len(tools), want)
		}
	}
}

// Session text read in an earlier turn is replayed to the model in the next
// one, so it still only proposes: a change asked for in the later turn waits
// for the person, even on Allow.
func TestAChangeInALaterTurnAfterSessionTextWasReadStillAsks(t *testing.T) {
	reading := HostTool{Name: "reet_get_factory_outcome", Title: "Read one outcome", Description: "Read it.", Parameters: json.RawMessage(`{"type":"object","properties":{"outcome_id":{"type":"string"}}}`), ReadOnly: true, Mode: "allow", SessionText: true}
	s, st, p, v, host, provider, _ := hostFixture(t, []HostTool{readTool, reading, writeTool},
		`reet_get_factory_outcome {"outcome_id":"abc"}`, "Read it.", `reet_stop_outcome {"outcome_id":"abc"}`, "I’ve asked to stop abc.")
	// The provider may read session text, so an earlier result is repeated to it.
	row, err := st.GafferProviderFor(p.UserID, v.Conversation.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	in := inputFrom(row)
	in.AllowStatus, in.AllowSessions, in.AllowCode = true, true, true
	if _, err = s.Save(p, in); err != nil {
		t.Fatal(err)
	}
	if first := ask(t, s, p, v, "a", "Read abc."); first.State != "completed" || len(first.Tools) != 1 || !first.Tools[0].SessionText {
		t.Fatalf("first turn: %+v", first)
	}
	provider.mu.Lock()
	before := len(provider.bodies)
	provider.mu.Unlock()
	second := ask(t, s, p, v, "b", "Carry on.")
	provider.mu.Lock()
	replayed := strings.Contains(strings.Join(provider.bodies[before:], " "), "RESULT_OF_reet_get_factory_outcome")
	provider.mu.Unlock()
	if !replayed {
		t.Fatal("the earlier result was not replayed, so this test shows nothing")
	}
	if got := host.made(); len(got) != 1 {
		t.Fatalf("a change ran on replayed session text: %v", got)
	}
	if len(second.Tools) != 1 || second.Tools[0].Name != "reet_stop_outcome" || second.Tools[0].State != "pending" || second.Tools[0].Mode != "ask" {
		t.Fatalf("the change should wait for the person: %+v", second.Tools)
	}
}

// cancellingHost cancels the answer while its tool is at work, as a person
// pressing Cancel answer does.
type cancellingHost struct {
	s            *Service
	uid          int64
	conversation string
}

func (h *cancellingHost) GafferTools(int64) []HostTool { return []HostTool{writeTool} }
func (h *cancellingHost) CallGafferTool(context.Context, int64, string, json.RawMessage) (string, error) {
	_ = h.s.CancelTurn(h.uid, h.conversation, strings.Repeat("a", 32))
	return `{"stopped":true}`, nil
}

// A tool that ran is on the turn and in the activity log even when the answer
// was cancelled while it ran.
func TestAToolThatRanDuringACancelIsStillRecorded(t *testing.T) {
	s, _, p, v, _, _, log := hostFixture(t, []HostTool{writeTool}, `reet_stop_outcome {"outcome_id":"x"}`, "Done.")
	s.SetToolHost(&cancellingHost{s: s, uid: p.UserID, conversation: v.Conversation.ID})
	turn := ask(t, s, p, v, "a", "Stop outcome x.")
	if turn.State != "cancelled" || len(turn.Tools) != 1 || turn.Tools[0].State != "ran" {
		t.Fatalf("turn: state=%s tools=%+v", turn.State, turn.Tools)
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if len(log.rows) != 1 || log.rows[0].action != "gaffer-tool-call" {
		t.Fatalf("audit: %+v", log.rows)
	}
}

// A model may ask for several of reet's tools in one answer. The first runs,
// the rest are dropped and the turn carries on: it does not fail.
func TestOfSeveralToolsAskedForAtOnceOnlyTheFirstRuns(t *testing.T) {
	s, _, p := settingsFixture(t)
	host := &fakeToolHost{tools: []HostTool{readTool, writeTool}, fail: map[string]string{}}
	s.SetToolHost(host)
	requests := 0
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		delta := map[string]any{"content": "One outcome is running."}
		if requests == 1 {
			call := func(i int, name string) map[string]any {
				return map[string]any{"index": i, "id": fmt.Sprintf("c%d", i), "type": "function", "function": map[string]any{"name": name, "arguments": "{}"}}
			}
			delta = map[string]any{"tool_calls": []any{call(0, "reet_list_factory"), call(1, "reet_stop_outcome")}}
		}
		raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": "stop"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", raw)
	}))
	t.Cleanup(fake.Close)
	turn := ask(t, s, p, conversationFixture(t, s, p, fake.URL), "a", "What is in the Factory?")
	if turn.State != "completed" || len(turn.Tools) != 1 || turn.Tools[0].Name != "reet_list_factory" || turn.Tools[0].State != "ran" {
		t.Fatalf("turn: state=%s error=%q tools=%+v", turn.State, turn.Error, turn.Tools)
	}
	if got := host.made(); len(got) != 1 || got[0] != "reet_list_factory {}" {
		t.Fatalf("calls made: %v", got)
	}
}

// A tool request the provider's output limit cut off is not run, and the
// turn says what happened in words that fit any tool.
func TestAToolRequestCutShortIsNotRunAndSaysSo(t *testing.T) {
	s, _, p := settingsFixture(t)
	host := &fakeToolHost{tools: []HostTool{writeTool}, fail: map[string]string{}}
	s.SetToolHost(host)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := map[string]any{"index": 0, "id": "c0", "type": "function", "function": map[string]any{"name": "reet_stop_outcome", "arguments": `{"outcome_id":"ab`}}
		raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{call}}, "finish_reason": "length"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", raw)
	}))
	t.Cleanup(fake.Close)
	turn := ask(t, s, p, conversationFixture(t, s, p, fake.URL), "a", "Stop abc.")
	if turn.State != "failed" || turn.Error != errToolRequestCut.Error() || len(turn.Tools) != 0 || len(host.made()) != 0 {
		t.Fatalf("turn: state=%s error=%q tools=%+v", turn.State, turn.Error, turn.Tools)
	}
}

// A skill's text is written by its author, not the person. Once a tool has put
// such text in front of the model, a change waits for the person even on
// Allow, in that turn and in a later one that repeats the result.
func TestAChangeAfterReadingOthersTextAsksEvenOnAllow(t *testing.T) {
	skill := HostTool{Name: "reet_get_skill", Title: "Read a skill", Description: "Read it.", Parameters: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`), ReadOnly: true, Mode: "allow", OthersText: true}
	s, _, p, v, host, _, _ := hostFixture(t, []HostTool{skill, writeTool},
		`reet_get_skill {"id":"x"}`, `reet_stop_outcome {"outcome_id":"abc"}`, "I’ve asked to stop abc.", `reet_stop_outcome {"outcome_id":"def"}`, "I’ve asked to stop def.")
	turn := ask(t, s, p, v, "a", "What does skill x do?")
	if turn.State != "completed" || len(turn.Tools) != 2 || !turn.Tools[0].OthersText || turn.Tools[1].State != "pending" || turn.Tools[1].Mode != "ask" {
		t.Fatalf("same turn: %+v", turn.Tools)
	}
	later := ask(t, s, p, v, "b", "Stop def.")
	if later.State != "completed" || len(later.Tools) != 1 || later.Tools[0].State != "pending" {
		t.Fatalf("later turn: %+v", later.Tools)
	}
	if got := host.made(); len(got) != 1 || !strings.HasPrefix(got[0], "reet_get_skill") {
		t.Fatalf("a change ran after a skill was read: %v", got)
	}
}
