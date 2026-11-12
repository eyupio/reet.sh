package gaffer

import (
	"encoding/json"
	"errors"
	"fmt"
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

func conversationFixture(t *testing.T, s *Service, p Principal, url string) ConversationView {
	t.Helper()
	row, err := s.Save(p, Input{Name: "Fake chat", Kind: "openai_compatible", BaseURL: url, LocalOnly: true, Enabled: true, Model: "fake-model", MakeDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateConversation(p.UserID, ConversationInput{ProviderID: row.ID})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func waitTurn(t *testing.T, s *Service, uid int64, c, id string) store.GafferTurn {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		turn, err := s.store.GafferTurnFor(uid, c, id)
		if err != nil {
			t.Fatal(err)
		}
		if turn.State != "running" {
			return turn
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("turn stayed running")
	return store.GafferTurn{}
}
func TestGafferConversationPersistsAnswersAndDeduplicatesRequests(t *testing.T) {
	s, _, p := settingsFixture(t)
	var calls atomic.Int32
	requests := make(chan string, 3)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
		}
		raw, _ := json.Marshal(in)
		requests <- string(raw)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Ey up. Kettle on.\"}}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":4}}\n\ndata: [DONE]\n\n")
	}))
	defer func() { s.Close(); fake.Close() }()
	v := conversationFixture(t, s, p, fake.URL)
	id := strings.Repeat("a", 32)
	input := TurnInput{ID: id, Revision: v.Conversation.Revision, Text: "Help me think this through"}
	if _, err := s.StartTurn(p.UserID, v.Conversation.ID, input); err != nil {
		t.Fatal(err)
	}
	first := waitTurn(t, s, p.UserID, v.Conversation.ID, id)
	if first.State != "completed" || first.Answer != "Ey up. Kettle on." || !first.UsageReported || first.InputTokens != 7 {
		t.Fatalf("answer %+v", first)
	}
	req := <-requests
	if strings.Contains(req, `"tools"`) || !strings.Contains(req, "Yorkshire") {
		t.Fatalf("unexpected request %s", req)
	}
	if _, err := s.StartTurn(p.UserID, v.Conversation.ID, input); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("duplicate provider call")
	}
	// A replacement service sees durable completed history, with no automatic call.
	s.Close()
	next := New(s.store, s.sealer)
	defer next.Close()
	if _, err := next.StartTurn(p.UserID, v.Conversation.ID, input); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("restart replay")
	}
	view, err := next.Conversation(p.UserID, v.Conversation.ID)
	if err != nil || len(view.Turns) != 1 {
		t.Fatalf("history: %v", err)
	}
	_, err = next.StartTurn(p.UserID, v.Conversation.ID, TurnInput{ID: strings.Repeat("b", 32), Revision: view.Conversation.Revision, Text: "What did I ask?"})
	if err != nil {
		t.Fatal(err)
	}
	waitTurn(t, next, p.UserID, v.Conversation.ID, strings.Repeat("b", 32))
	req = <-requests
	if !strings.Contains(req, "Help me think this through") || !strings.Contains(req, "Kettle on.") {
		t.Fatalf("history omitted: %s", req)
	}
}
func TestGafferTurnCancellationPolicyChangeAndShutdownPreservePartialAnswers(t *testing.T) {
	for _, action := range []string{"cancel", "pause", "shutdown", "demote"} {
		t.Run(action, func(t *testing.T) {
			s, st, p := settingsFixture(t)
			started := make(chan struct{})
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", strings.Repeat("partial ", 600))
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}))
			defer func() { s.Close(); fake.Close() }()
			v := conversationFixture(t, s, p, fake.URL)
			id := strings.Repeat("a", 32)
			if _, err := s.StartTurn(p.UserID, v.Conversation.ID, TurnInput{ID: id, Revision: 1, Text: "Take your time"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("provider never started")
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				partial, err := st.GafferTurnFor(p.UserID, v.Conversation.ID, id)
				if err != nil {
					t.Fatal(err)
				}
				if partial.Answer != "" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("partial answer was not persisted")
				}
				time.Sleep(10 * time.Millisecond)
			}
			switch action {
			case "cancel":
				if err := s.CancelTurn(p.UserID, v.Conversation.ID, id); err != nil {
					t.Fatal(err)
				}
			case "pause":
				rows, _ := s.List(p)
				if err := s.SetEnabled(p, rows[0].ID, rows[0].Revision, false); err != nil {
					t.Fatal(err)
				}
			case "shutdown":
				s.Close()
			case "demote":
				if err := st.UpdateUserRole(p.UserID, "viewer"); err != nil {
					t.Fatal(err)
				}
			}
			turn := waitTurn(t, s, p.UserID, v.Conversation.ID, id)
			expected := map[string]string{"cancel": "cancelled", "pause": "failed", "shutdown": "interrupted", "demote": "failed"}[action]
			if turn.State != expected {
				t.Fatalf("state %+v", turn)
			}
			if turn.Answer == "" {
				t.Fatal("partial answer lost")
			}
		})
	}
}
func TestGafferSessionConversationsNeverReadWorkerHistory(t *testing.T) {
	s, st, p := settingsFixture(t)
	requests := make(chan string, 4)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		raw, _ := json.Marshal(in)
		requests <- string(raw)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"I only have this Gaffer conversation.\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer func() { s.Close(); fake.Close() }()
	general := conversationFixture(t, s, p, fake.URL)
	agent, _, err := st.EnrollSyncAgent(p.UserID, "test-machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps, err := st.BeginRemoteConnectionFor(p.UserID, agent.ID, strings.Repeat("1", 32), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready, ACP: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		session, _, err := st.ReserveTerminalCreateFor(p.UserID, agent.ID, strings.Repeat(string(rune('2'+i)), 32), caps.Generation, terminalproto.Create{Title: "Worker secret title", Tool: "opencode", Model: "worker-model", Structured: true, Dir: "/secret/worktree", Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AppendTerminalChatEventFor(p.UserID, session.ID, 1, time.Now().UnixMilli(), []byte(`{"kind":"user","text":"worker-secret-context"}`)); err != nil {
			t.Fatal(err)
		}
		v, err := s.CreateConversation(p.UserID, ConversationInput{SessionID: session.ID, ProviderID: general.Conversation.ProviderID})
		if err != nil {
			t.Fatal(err)
		}
		if v.Conversation.ID == general.Conversation.ID {
			t.Fatal("session reused general conversation")
		}
		id := strings.Repeat(string(rune('a'+i)), 32)
		if _, err := s.StartTurn(p.UserID, v.Conversation.ID, TurnInput{ID: id, Revision: 1, Text: fmt.Sprintf("Gaffer session %d only", i)}); err != nil {
			t.Fatal(err)
		}
		waitTurn(t, s, p.UserID, v.Conversation.ID, id)
		req := <-requests
		if strings.Contains(req, "worker-secret-context") || strings.Contains(req, "worker-model") || strings.Contains(req, "/secret/worktree") || strings.Contains(req, "Worker secret title") || i == 1 && strings.Contains(req, "Gaffer session 0 only") {
			t.Fatalf("scope leaked %s", req)
		}
	}
	other, err := st.CreateUser("no-access@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Conversation(other.ID, general.Conversation.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-account read %v", err)
	}
}
func TestGafferRejectsUnsolicitedToolsAndIncompleteStreams(t *testing.T) {
	for _, answer := range []string{
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"a\",\"function\":{\"name\":\"run_shell\",\"arguments\":\"{}\"}}]}}]}\n\ndata: [DONE]\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"incomplete\"}}]}\n\n",
	} {
		s, _, p := settingsFixture(t)
		fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, answer) }))
		v := conversationFixture(t, s, p, fake.URL)
		id := strings.Repeat("d", 32)
		if _, err := s.StartTurn(p.UserID, v.Conversation.ID, TurnInput{ID: id, Revision: 1, Text: "Help"}); err != nil {
			t.Fatal(err)
		}
		turn := waitTurn(t, s, p.UserID, v.Conversation.ID, id)
		if turn.State != "failed" {
			t.Fatalf("untrusted stream accepted: %+v", turn)
		}
		fake.Close()
	}
}

func TestGafferReplyLimitPreservesValidTextAndReportsTruncation(t *testing.T) {
	s, _, p := settingsFixture(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", strings.Repeat("☕", store.MaxGafferAnswerBytes/3+10))
	}))
	defer func() { s.Close(); fake.Close() }()
	v := conversationFixture(t, s, p, fake.URL)
	id := strings.Repeat("e", 32)
	if _, err := s.StartTurn(p.UserID, v.Conversation.ID, TurnInput{ID: id, Revision: 1, Text: "A long answer"}); err != nil {
		t.Fatal(err)
	}
	turn := waitTurn(t, s, p.UserID, v.Conversation.ID, id)
	if turn.State != "completed" || !turn.Cut || len(turn.Answer) > store.MaxGafferAnswerBytes || !utf8.ValidString(turn.Answer) {
		t.Fatalf("bad bounded reply state=%s cut=%v bytes=%d", turn.State, turn.Cut, len(turn.Answer))
	}
}

// The OpenAI adapter's prefix cache is routed by prompt_cache_key, so each
// Gaffer conversation sends its own durable id and repeated turns land on the
// same cache shard.
func TestGafferChatSendsTheConversationAsPromptCacheKey(t *testing.T) {
	s, _, p := settingsFixture(t)
	requests := make(chan string, 1)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
		}
		raw, _ := json.Marshal(in)
		requests <- string(raw)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Ey up.\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer func() { s.Close(); fake.Close() }()
	row, err := s.Save(p, Input{Name: "OpenAI chat", Kind: "openai", BaseURL: fake.URL, LocalOnly: true, Enabled: true, Model: "fake-model", MakeDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateConversation(p.UserID, ConversationInput{ProviderID: row.ID})
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("b", 32)
	if _, err := s.StartTurn(p.UserID, v.Conversation.ID, TurnInput{ID: id, Revision: v.Conversation.Revision, Text: "Hello"}); err != nil {
		t.Fatal(err)
	}
	if turn := waitTurn(t, s, p.UserID, v.Conversation.ID, id); turn.State != "completed" {
		t.Fatalf("turn %+v", turn)
	}
	if req := <-requests; !strings.Contains(req, `"prompt_cache_key":"`+v.Conversation.ID+`"`) {
		t.Fatalf("request %s", req)
	}
}
