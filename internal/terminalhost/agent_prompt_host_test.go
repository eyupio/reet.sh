//go:build linux || darwin

package terminalhost

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// agentSession makes a real tmux session and records it as one the given
// agent was started in, without starting any agent.
func agentSession(t *testing.T, m *Manager, n int, tool string) (id, tty string) {
	t.Helper()
	ctx := context.Background()
	id = testSessionID(n)
	if _, err := m.Create(ctx, testRequestID(n), id, terminalproto.Create{Title: "Agent", Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	r, err := m.loadRegistry()
	if err == nil {
		s := r.Sessions[id]
		s.Tool = tool
		r.Sessions[id] = s
		err = m.saveRegistry(r)
	}
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return id, paneTTY(t, m, id)
}

// draw replaces the pane's screen with text, as an agent redraws its own.
func draw(t *testing.T, tty, text string) {
	t.Helper()
	if err := os.WriteFile(tty, []byte("\x1b[2J\x1b[H"+strings.ReplaceAll(strings.TrimLeft(text, "\n"), "\n", "\r\n")), 0); err != nil {
		t.Fatal(err)
	}
}

func bellOf(t *testing.T, m *Manager, id string) int64 {
	t.Helper()
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == id {
			return s.BellAt
		}
	}
	t.Fatal("session not listed")
	return 0
}

// waitBell reconciles until the session's bell time passes after, or the
// deadline does.
func waitBell(t *testing.T, m *Manager, id string, after int64, within time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if at := bellOf(t, m, id); at > after || time.Now().After(deadline) {
			return at
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// An agent that puts a question on its screen without ringing the bell is
// recorded as having asked, once for each question.
func TestAQuestionOnAnAgentsScreenCountsAsABell(t *testing.T) {
	m, _ := managerFixture(t)
	id, tty := agentSession(t, m, 51, "claude")

	draw(t, tty, claudeWorking)
	if at := waitBell(t, m, id, 0, 3*time.Second); at != 0 {
		t.Fatalf("a working agent was recorded as asking: %d", at)
	}
	before := time.Now().UnixMilli()
	draw(t, tty, claudeCommandPrompt)
	first := waitBell(t, m, id, 0, 6*time.Second)
	if first < before {
		t.Fatalf("the question was not recorded: bell %d, drawn at %d", first, before)
	}
	// The question stays on screen, redrawn as an agent redraws it: it is read
	// again and is not recorded again.
	read := m.prompts[id].at
	time.Sleep(1100 * time.Millisecond)
	draw(t, tty, claudeCommandPrompt)
	if at := waitBell(t, m, id, first, 4*time.Second); at != first {
		t.Fatalf("a question still on screen was recorded again: %d then %d", first, at)
	}
	if !m.prompts[id].at.After(read) {
		t.Fatal("the redrawn screen was never read, so nothing was shown")
	}
	// It is answered, the agent works, and asks again: a new question.
	draw(t, tty, claudeWorking)
	if at := waitBell(t, m, id, first, 3*time.Second); at != first {
		t.Fatalf("an answered question was recorded again: %d then %d", first, at)
	}
	draw(t, tty, claudeCommandPrompt)
	if second := waitBell(t, m, id, first, 6*time.Second); second <= first {
		t.Fatalf("a second question was not recorded: %d then %d", first, second)
	}
}

// A shell, and an agent reet has no rules for, are never read: the same text
// on their screens records nothing.
func TestOnlyAnAgentWithRulesHasItsScreenRead(t *testing.T) {
	m, _ := managerFixture(t)
	for n, tool := range map[int]string{52: "", 53: "cline"} {
		id, tty := agentSession(t, m, n, tool)
		draw(t, tty, claudeCommandPrompt)
		if at := waitBell(t, m, id, 0, 3*time.Second); at != 0 {
			t.Fatalf("tool %q: a question was recorded for a session whose screen is not read: %d", tool, at)
		}
		if _, read := m.prompts[id]; read {
			t.Fatalf("tool %q: the screen was read", tool)
		}
	}
}

// What is on the screen stays on the machine: the session's snapshot carries
// a time and nothing of the text.
func TestAQuestionOnScreenCarriesNoOutput(t *testing.T) {
	m, _ := managerFixture(t)
	id, tty := agentSession(t, m, 54, "codex")
	secret := "SECRET_COMMAND_TEXT_DO_NOT_LEAK_13579"
	draw(t, tty, strings.Replace(codexCommandPrompt, "make migrate", secret, 1))
	if at := waitBell(t, m, id, 0, 6*time.Second); at == 0 {
		t.Fatal("the question was not recorded")
	}
	list, err := m.List(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatal(err, list)
	}
	s := list[0]
	raw, err := json.Marshal(terminalproto.SessionSnapshot{ID: s.ID, Title: s.Title, State: s.State, ActivityAt: s.ActivityAt, BellAt: s.BellAt}.ForFeatures(terminalproto.AttentionFeature))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := os.ReadFile(m.registryPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(registry), secret) {
		t.Fatal("screen text reached the snapshot or the registry")
	}
}

// A screen that keeps changing is read at most every promptReadEvery, and a
// settled one not at all until it changes.
func TestAScreenIsReadSparingly(t *testing.T) {
	m, _ := managerFixture(t)
	id, tty := agentSession(t, m, 55, "claude")
	draw(t, tty, claudeIdle)
	deadline := time.Now().Add(5 * time.Second)
	for !m.prompts[id].settled {
		if time.Now().After(deadline) {
			t.Fatal("an idle screen was never read as settled")
		}
		bellOf(t, m, id)
		time.Sleep(100 * time.Millisecond)
	}
	read := m.prompts[id].at
	// Long enough for the least gap between two reads to pass several times.
	for i := 0; i < 20; i++ {
		bellOf(t, m, id)
		time.Sleep(4 * promptReadAgain / 20)
	}
	if !m.prompts[id].at.Equal(read) {
		t.Fatal("a settled screen was read again though nothing changed")
	}
}
