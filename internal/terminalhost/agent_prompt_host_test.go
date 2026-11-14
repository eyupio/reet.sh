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

// promptFixture is a manager whose gaps between two reads of a screen are
// too short to notice, so a test waits for tmux and never for the clock. The
// rules about when a screen is read are the same.
func promptFixture(t *testing.T) *Manager {
	t.Helper()
	m, _ := managerFixture(t)
	m.promptEvery, m.promptAgain = time.Millisecond, time.Millisecond
	return m
}

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

// draw replaces the pane's screen with text, as an agent redraws its own, and
// returns once tmux shows it: mark is a piece of text that the screen before
// did not have.
func draw(t *testing.T, m *Manager, id, tty, text, mark string) {
	t.Helper()
	if err := os.WriteFile(tty, []byte("\x1b[2J\x1b[H"+strings.ReplaceAll(strings.TrimLeft(text, "\n"), "\n", "\r\n")), 0); err != nil {
		t.Fatal(err)
	}
	pane, err := m.command(context.Background(), 64, "list-panes", "-t", "="+m.name(id), "-F", "#{pane_id}")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		screen, err := m.command(context.Background(), promptScreenBytes, "capture-pane", "-p", "-t", strings.TrimSpace(string(pane)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(screen), mark) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("tmux never showed %q", mark)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// bellOf reconciles once and returns the session's bell time.
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

// An agent that puts a question on its screen without ringing the bell is
// recorded as having asked, once for each question.
func TestAQuestionOnAnAgentsScreenCountsAsABell(t *testing.T) {
	m := promptFixture(t)
	id, tty := agentSession(t, m, 51, "claude")
	const working, question = "Running tests", "Do you want to proceed?"

	draw(t, m, id, tty, claudeWorking, working)
	if at := bellOf(t, m, id); at != 0 {
		t.Fatalf("a working agent was recorded as asking: %d", at)
	}
	if watch, read := m.prompts[id]; !read || watch.waiting {
		t.Fatalf("the working screen was not read as one without a question: %+v, read %v", watch, read)
	}
	before := time.Now().UnixMilli()
	draw(t, m, id, tty, claudeCommandPrompt, question)
	first := bellOf(t, m, id)
	if first < before {
		t.Fatalf("the question was not recorded: bell %d, drawn at %d", first, before)
	}
	// The question stays on screen, redrawn as an agent redraws it: it is read
	// again and is not recorded again.
	read := m.prompts[id].at
	draw(t, m, id, tty, strings.Replace(claudeCommandPrompt, "tests now", "tests again", 1), "tests again")
	if at := bellOf(t, m, id); at != first {
		t.Fatalf("a question still on screen was recorded again: %d then %d", first, at)
	}
	if watch := m.prompts[id]; !watch.at.After(read) || !watch.waiting {
		t.Fatalf("the redrawn question was not read as a question: %+v", watch)
	}
	// It is answered, the agent works, and asks again: a new question.
	draw(t, m, id, tty, claudeWorking, working)
	if at := bellOf(t, m, id); at != first {
		t.Fatalf("an answered question was recorded again: %d then %d", first, at)
	}
	if m.prompts[id].waiting {
		t.Fatal("the answered screen was not read")
	}
	draw(t, m, id, tty, claudeCommandPrompt, question)
	if second := bellOf(t, m, id); second <= first {
		t.Fatalf("a second question was not recorded: %d then %d", first, second)
	}
}

// A shell, and an agent reet has no rules for, are never read: the same text
// on their screens records nothing.
func TestOnlyAnAgentWithRulesHasItsScreenRead(t *testing.T) {
	m := promptFixture(t)
	for n, tool := range map[int]string{52: "", 53: "cline"} {
		id, tty := agentSession(t, m, n, tool)
		draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
		if at := bellOf(t, m, id); at != 0 {
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
	m := promptFixture(t)
	id, tty := agentSession(t, m, 54, "codex")
	secret := "SECRET_COMMAND_TEXT_DO_NOT_LEAK_13579"
	draw(t, m, id, tty, strings.Replace(codexCommandPrompt, "make migrate", secret, 1), secret)
	if at := bellOf(t, m, id); at == 0 {
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

// A settled screen is not read again until it changes, however often the
// sessions are reconciled and however short the gap between two reads.
func TestAScreenIsReadSparingly(t *testing.T) {
	m := promptFixture(t)
	id, tty := agentSession(t, m, 55, "claude")
	draw(t, m, id, tty, claudeIdle, "All 42 tests pass")
	// tmux counts activity in whole seconds, so this waits for the second the
	// screen was drawn in to be over.
	deadline := time.Now().Add(5 * time.Second)
	for !m.prompts[id].settled {
		if time.Now().After(deadline) {
			t.Fatal("an idle screen was never read as settled")
		}
		bellOf(t, m, id)
		time.Sleep(20 * time.Millisecond)
	}
	read := m.prompts[id].at
	for i := 0; i < 20; i++ {
		bellOf(t, m, id)
		time.Sleep(2 * m.promptAgain)
	}
	if !m.prompts[id].at.Equal(read) {
		t.Fatal("a settled screen was read again though nothing changed")
	}
	// A change is read.
	draw(t, m, id, tty, claudeWorking, "Running tests")
	bellOf(t, m, id)
	if !m.prompts[id].at.After(read) {
		t.Fatal("a changed screen was not read")
	}
}

// A screen that was just read is not read again inside the gap, whatever is
// drawn on it meanwhile: an agent's spinner does not have its screen captured
// on every reconcile.
func TestAScreenIsNotReadAgainInsideTheGap(t *testing.T) {
	m, _ := managerFixture(t)
	m.promptEvery, m.promptAgain = time.Hour, time.Hour
	id, tty := agentSession(t, m, 56, "claude")
	draw(t, m, id, tty, claudeWorking, "Running tests")
	bellOf(t, m, id)
	watch, read := m.prompts[id]
	if !read {
		t.Fatal("the first look did not read the screen")
	}
	draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
	for i := 0; i < 5; i++ {
		if at := bellOf(t, m, id); at != 0 {
			t.Fatalf("the screen was read inside the gap: bell %d", at)
		}
	}
	if !m.prompts[id].at.Equal(watch.at) {
		t.Fatal("the screen was read inside the gap")
	}
}
