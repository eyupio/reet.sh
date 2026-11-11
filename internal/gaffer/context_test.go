package gaffer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func contextSessionFixture(t *testing.T, st *store.Store, p Principal) store.TerminalSession {
	t.Helper()
	agent, _, err := st.EnrollSyncAgent(p.UserID, "context-machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps, err := st.BeginRemoteConnectionFor(p.UserID, agent.ID, strings.Repeat("1", 32), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready, ACP: true})
	if err != nil {
		t.Fatal(err)
	}
	row, _, err := st.ReserveTerminalCreateFor(p.UserID, agent.ID, strings.Repeat("2", 32), caps.Generation, terminalproto.Create{Title: "WORKER_TITLE_SECRET", Tool: "opencode", Model: "worker-model", Structured: true, Dir: "/PRIVATE_SOURCE_PATH", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestContextArchiveRevokesSharingAndRemovesRetainedFleetCounts(t *testing.T) {
	s, st, p := settingsFixture(t)
	row := contextSessionFixture(t, st, p)
	other, err := st.CreateUser("other-fleet@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	contextSessionFixture(t, st, Principal{UserID: other.ID})
	client, err := st.CreateMCPConnection(p.UserID, "Context client", "read", "context-archive-hash", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetMCPEnabled(p.UserID, true); err != nil {
		t.Fatal(err)
	}
	subject := fmt.Sprint(client.ID)
	for _, scope := range []string{"", row.ID} {
		if _, err = s.ConfigureContext(p.UserID, "mcp", subject, scope, ContextInput{ContextFlags: store.ContextFlags{Status: true}}); err != nil {
			t.Fatal(err)
		}
	}
	fleet, err := s.ReadMCPContext(p.UserID, "mcp", subject, "")
	if err != nil || fleet.Fleet == nil || fleet.Fleet.Machines != 1 || fleet.Fleet.Sessions["creating"] != 1 {
		t.Fatal("fleet included another account or lost own session", fleet, err)
	}
	raw, _ := json.Marshal(fleet)
	if strings.Contains(string(raw), "WORKER_TITLE_SECRET") || strings.Contains(string(raw), "PRIVATE_SOURCE_PATH") {
		t.Fatal("fleet included private text", string(raw))
	}
	if err = st.ObserveTerminalStateFor(p.UserID, row.AgentID, row.ID, 1, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	if err = st.ObserveTerminalStateFor(p.UserID, row.AgentID, row.ID, 1, terminalproto.Exited, new(int)); err != nil {
		t.Fatal(err)
	}
	if err = st.ArchiveTerminalFor(p.UserID, row.ID); err != nil {
		t.Fatal(err)
	}
	grant, err := st.ContextGrantFor(p.UserID, "mcp", subject, row.ID)
	if err != nil || grant.Any() || grant.Revision != 2 {
		t.Fatal("archived session retained a grant", grant, err)
	}
	if _, err = s.ReadMCPContext(p.UserID, "mcp", subject, row.ID); !errors.Is(err, ErrContextForbidden) {
		t.Fatal("archived context remained readable", err)
	}
	scopes, err := s.MCPContextAccess(p.UserID, "mcp", subject)
	if err != nil || len(scopes) != 1 || scopes[0].SessionID != "" {
		t.Fatal("archived scope remained listed", scopes, err)
	}
	fleet, err = s.ReadMCPContext(p.UserID, "mcp", subject, "")
	if err != nil || len(fleet.Fleet.Sessions) != 0 {
		t.Fatal("archive retained activity count", fleet, err)
	}
}

func shareContext(t *testing.T, s *Service, uid int64, c ConversationView, flags store.ContextFlags) ConversationView {
	t.Helper()
	v, err := s.ContextSettings(uid, "gaffer", c.Conversation.ID, c.Conversation.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureContext(uid, "gaffer", c.Conversation.ID, c.Conversation.SessionID, ContextInput{ContextFlags: flags, Revision: v.Grant.Revision, ProviderID: v.ProviderID, ProviderRevision: v.ProviderRevision, Model: v.Model})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.Conversation(uid, c.Conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGafferContextRequiresExplicitSharingAndDropsRevokedDerivedHistory(t *testing.T) {
	s, st, p := settingsFixture(t)
	row := contextSessionFixture(t, st, p)
	for i, raw := range []string{`{"kind":"user","text":"WORKER_PRIVATE_TEXT"}`, `{"kind":"agent","messageId":"m1","text":"Agent progress"}`, `{"kind":"tool","toolKind":"execute","status":"completed","text":"RAW_TOOL_SECRET","command":"RAW_COMMAND_SECRET","diff":"RAW_DIFF_SECRET"}`, `{"kind":"thought","text":"REASONING_SECRET"}`, `{"kind":"permission","text":"APPROVAL_SECRET"}`} {
		if err := st.AppendTerminalChatEventFor(p.UserID, row.ID, int64(i+1), time.Now().UnixMilli(), []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	requests := make(chan string, 4)
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requests <- string(raw)
		answer := "Public answer"
		if calls.Add(1) == 2 {
			answer = "PRIVATE_DERIVED_ANSWER"
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", answer)
	}))
	defer fake.Close()
	provider, err := s.Save(p, Input{Name: "Context fake", Kind: "openai_compatible", BaseURL: fake.URL, LocalOnly: true, Enabled: true, Model: "fake-model", AllowStatus: true, AllowSessions: true, AllowCode: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(p.UserID, ConversationInput{SessionID: row.ID, ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	run := func(id, text string) store.GafferTurn {
		t.Helper()
		_, err := s.StartTurn(p.UserID, c.Conversation.ID, TurnInput{ID: strings.Repeat(id, 32), Revision: c.Conversation.Revision, Text: text})
		if err != nil {
			t.Fatal(err)
		}
		turn := waitTurn(t, s, p.UserID, c.Conversation.ID, strings.Repeat(id, 32))
		if turn.State != "completed" {
			t.Fatal(turn.Error)
		}
		c, err = s.Conversation(p.UserID, c.Conversation.ID)
		if err != nil {
			t.Fatal(err)
		}
		return turn
	}
	if turn := run("a", "Typed only"); turn.Context != nil {
		t.Fatal("implicit context", turn.Context)
	}
	first := <-requests
	assertFollowupGuidance := func(payload string) {
		t.Helper()
		if !strings.Contains(payload, "Look after this session") || !strings.Contains(payload, "explicitly configures") {
			t.Fatal("ordinary chat omitted the separately granted follow-up path")
		}
		var sent struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal([]byte(payload), &sent); err != nil || len(sent.Tools) != 0 {
			t.Fatal("ordinary chat acquired tools without a grant", err)
		}
	}
	assertFollowupGuidance(first)
	if strings.Contains(first, "WORKER_") || strings.Contains(first, "<reet_context>") {
		t.Fatal("ungranted data", first)
	}
	oldRevision := c.Conversation.Revision
	c = shareContext(t, s, p.UserID, c, store.ContextFlags{Status: true, History: true, Text: true})
	if _, err = s.StartTurn(p.UserID, c.Conversation.ID, TurnInput{ID: strings.Repeat("e", 32), Revision: oldRevision, Text: "Stale window"}); !errors.Is(err, store.ErrGafferConflict) {
		t.Fatal("stale window sent with changed sharing", err)
	}
	used := run("b", "Read the selected session")
	second := <-requests
	assertFollowupGuidance(second)
	if !strings.Contains(second, "WORKER_PRIVATE_TEXT") || used.Context == nil || used.Context.Digest == "" || used.Context.LastSeq != 5 {
		t.Fatal("missing permitted evidence", second, used.Context)
	}
	for _, secret := range []string{"RAW_TOOL_SECRET", "RAW_COMMAND_SECRET", "RAW_DIFF_SECRET", "REASONING_SECRET", "APPROVAL_SECRET", "PRIVATE_SOURCE_PATH"} {
		if strings.Contains(second, secret) {
			t.Fatal("excluded field", secret, second)
		}
	}
	c = shareContext(t, s, p.UserID, c, store.ContextFlags{})
	run("c", "Sharing is off now")
	third := <-requests
	if strings.Contains(third, "PRIVATE_DERIVED_ANSWER") || strings.Contains(third, "WORKER_PRIVATE_TEXT") || strings.Contains(third, "Read the selected session") {
		t.Fatal("revoked context was laundered through Gaffer history", third)
	}
	if calls.Load() != 3 {
		t.Fatal("implicit retry", calls.Load())
	}
	s.Close()
	reopened, err := store.Open(st.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	receipt, err := reopened.GafferTurnFor(p.UserID, c.Conversation.ID, strings.Repeat("b", 32))
	if err != nil || receipt.Context == nil || receipt.Context.Digest != used.Context.Digest {
		t.Fatal("lost context receipt", receipt, err)
	}
}

func TestGafferContextProjectionIsBoundedAndSeparatesTextFromMetadata(t *testing.T) {
	s, st, p := settingsFixture(t)
	row := contextSessionFixture(t, st, p)
	provider, err := s.Save(p, Input{Name: "Projection", Kind: "openai_compatible", BaseURL: "http://127.0.0.1:1", LocalOnly: true, Enabled: true, Model: "fake-model", AllowStatus: true, AllowSessions: true, AllowCode: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(p.UserID, ConversationInput{SessionID: row.ID, ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 60; i++ {
		raw, _ := json.Marshal(map[string]any{"kind": "agent", "text": strings.Repeat("界", 3000) + " WORKER_PRIVATE_TEXT", "command": "COMMAND_SECRET", "path": "PATH_SECRET"})
		if err = st.AppendTerminalChatEventFor(p.UserID, row.ID, int64(i), time.Now().UnixMilli(), raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, flags := range []store.ContextFlags{{Status: true}, {History: true}, {History: true, Text: true}} {
		x, err := s.PreviewContext(p.UserID, "gaffer", c.Conversation.ID, row.ID, flags)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(x)
		if len(raw) > MaxContextBytes || !utf8.Valid(raw) {
			t.Fatal("unbounded or broken UTF-8", len(raw))
		}
		if strings.Contains(string(raw), "COMMAND_SECRET") || strings.Contains(string(raw), "PATH_SECRET") {
			t.Fatal("raw payload forwarded")
		}
		if !flags.Text && (strings.Contains(string(raw), "界") || strings.Contains(string(raw), "WORKER_TITLE_SECRET")) {
			t.Fatal("metadata shared text", string(raw))
		}
		if flags.History && (x.History == nil || !x.History.Truncated || x.History.LastRetainedSeq != 60 || len(x.History.Events) > 20) {
			t.Fatal("missing retention bounds", x.History)
		}
	}
}

func TestGafferContextGrantBindsTheProviderIdentityNotJustModelAndRevision(t *testing.T) {
	s, _, p := settingsFixture(t)
	a, err := s.Save(p, Input{Name: "Provider A", Kind: "openai_compatible", BaseURL: "http://127.0.0.1:1", LocalOnly: true, Enabled: true, Model: "same", AllowStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Save(p, Input{Name: "Provider B", Kind: "openai_compatible", BaseURL: "http://127.0.0.1:2", LocalOnly: true, Enabled: true, Model: "same", AllowStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(p.UserID, ConversationInput{ProviderID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.ContextSettings(p.UserID, "gaffer", c.Conversation.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureConversation(p.UserID, c.Conversation.ID, c.Conversation.Revision, ConversationInput{ProviderID: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureContext(p.UserID, "gaffer", c.Conversation.ID, "", ContextInput{ContextFlags: store.ContextFlags{Status: true}, Revision: v.Grant.Revision, ProviderID: v.ProviderID, ProviderRevision: v.ProviderRevision, Model: v.Model})
	if !errors.Is(err, store.ErrGafferConflict) {
		t.Fatal("stale permission moved to another provider", err)
	}
}

func TestGafferContextRevocationStopsAnActiveAnswerWithoutRetry(t *testing.T) {
	s, _, p := settingsFixture(t)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Partial answer\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	t.Cleanup(func() { s.Close(); fake.Close() })
	provider, err := s.Save(p, Input{Name: "Revocable context", Kind: "openai_compatible", BaseURL: fake.URL, LocalOnly: true, Enabled: true, Model: "fake-model", AllowStatus: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(p.UserID, ConversationInput{ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	c = shareContext(t, s, p.UserID, c, store.ContextFlags{Status: true})
	in := TurnInput{ID: strings.Repeat("d", 32), Revision: c.Conversation.Revision, Text: "Check progress"}
	if _, err = s.StartTurn(p.UserID, c.Conversation.ID, in); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider did not start")
	}
	shareContext(t, s, p.UserID, c, store.ContextFlags{})
	turn := waitTurn(t, s, p.UserID, c.Conversation.ID, in.ID)
	if turn.State != "failed" || turn.Context == nil || !strings.Contains(turn.Error, "settings changed") {
		t.Fatal("revocation did not stop the turn", turn)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("provider call outlived revocation")
	}
	if _, err = s.StartTurn(p.UserID, c.Conversation.ID, in); err != nil {
		t.Fatal("receipt reconciliation failed", err)
	}
	if calls.Load() != 1 {
		t.Fatal("revoked turn retried", calls.Load())
	}
}

func TestGafferContextMasksRecognisedCredentialsAndKeepsUTF8(t *testing.T) {
	text := "api_key=ASSIGNMENT_SECRET sk-proj-abcdefghijklmnop https://name:URL_SECRET@example.test -----BEGIN PRIVATE KEY-----\nPRIVATE_KEY_SECRET\n-----END PRIVATE KEY-----"
	masked, _ := contextText(text, 4096)
	for _, secret := range []string{"ASSIGNMENT_SECRET", "sk-proj-abcdefghijklmnop", "URL_SECRET", "PRIVATE_KEY_SECRET"} {
		if strings.Contains(masked, secret) {
			t.Fatal("recognised credential remained", secret, masked)
		}
	}
	x, cut := contextText(strings.Repeat("界", 1000), 100)
	if !cut || !utf8.ValidString(x) || len(x) > 100 {
		t.Fatal("bad UTF-8 clipping", len(x), cut)
	}
}

// A plan or design is longer than one short history event. The snapshot
// carries the working model's latest reply at length, so the stage review can
// read the deliverable; it needs text sharing, is scrubbed and bounded, and
// holds only what was said since the model was last sent a message.
func TestGafferContextCarriesTheWorkersLatestReplyAtLength(t *testing.T) {
	s, st, p := settingsFixture(t)
	row := contextSessionFixture(t, st, p)
	provider, err := s.Save(p, Input{Name: "Reply", Kind: "openai_compatible", BaseURL: "http://127.0.0.1:1", LocalOnly: true, Enabled: true, Model: "fake-model", AllowStatus: true, AllowSessions: true, AllowCode: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(p.UserID, ConversationInput{SessionID: row.ID, ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	plan := "PLAN_START files to change: " + strings.Repeat("web/src/lib/Factory.svelte, ", 150) + "approach and risks. token=abcdefgh12345678 PLAN_END"
	events := []map[string]any{
		{"kind": "agent", "text": "OLD_TURN_TEXT from before the instruction."},
		{"kind": "user", "text": "Write the plan."},
		{"kind": "agent", "text": plan},
		{"kind": "tool", "toolKind": "read", "status": "completed"},
		{"kind": "agent", "text": "Plan written. No code changed; stopping here."},
		{"kind": "status", "state": "idle"},
	}
	for i, e := range events {
		raw, _ := json.Marshal(e)
		if err = st.AppendTerminalChatEventFor(p.UserID, row.ID, int64(i+1), time.Now().UnixMilli(), raw); err != nil {
			t.Fatal(err)
		}
	}
	x, err := s.PreviewContext(p.UserID, "gaffer", c.Conversation.ID, row.ID, store.ContextFlags{History: true, Text: true})
	if err != nil {
		t.Fatal(err)
	}
	r := x.LatestReply
	if r == nil || r.FromSeq != 3 || r.ToSeq != 5 || r.StartOmitted ||
		!strings.Contains(r.Text, "PLAN_START") || !strings.Contains(r.Text, "PLAN_END") || !strings.HasSuffix(r.Text, "stopping here.") {
		t.Fatalf("latest reply: %+v", r)
	}
	if strings.Contains(r.Text, "OLD_TURN_TEXT") || strings.Contains(r.Text, "abcdefgh12345678") {
		t.Fatalf("an earlier turn or a credential is in the reply: %q", r.Text)
	}
	// The short history event alone would have cut the plan off.
	for _, e := range x.History.Events {
		if strings.Contains(e.Text, "PLAN_START") && strings.Contains(e.Text, "PLAN_END") {
			t.Fatal("the history event was expected to be cut short")
		}
	}
	if raw, _ := json.Marshal(x); len(raw) > MaxContextBytes {
		t.Fatalf("snapshot is %d bytes", len(raw))
	}
	// Without text sharing there is no reply text at all.
	meta, err := s.PreviewContext(p.UserID, "gaffer", c.Conversation.ID, row.ID, store.ContextFlags{History: true})
	if err != nil || meta.LatestReply != nil {
		t.Fatalf("metadata-only snapshot carried the reply: %+v %v", meta.LatestReply, err)
	}
	// A reply longer than the limit keeps its end and says the start is missing.
	raw, _ := json.Marshal(map[string]any{"kind": "agent", "text": "LONG_START " + strings.Repeat("界", 5000) + " LONG_END"})
	if err = st.AppendTerminalChatEventFor(p.UserID, row.ID, 7, time.Now().UnixMilli(), raw); err != nil {
		t.Fatal(err)
	}
	long, err := s.PreviewContext(p.UserID, "gaffer", c.Conversation.ID, row.ID, store.ContextFlags{History: true, Text: true})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(long)
	if r = long.LatestReply; r == nil || !r.StartOmitted || len(r.Text) > MaxLatestReplyBytes || !strings.HasSuffix(r.Text, "LONG_END") || strings.Contains(r.Text, "PLAN_START") || !utf8.Valid(out) || len(out) > MaxContextBytes {
		t.Fatalf("long reply: omitted=%v len=%d snapshot=%d", r != nil && r.StartOmitted, len(r.Text), len(out))
	}
}
