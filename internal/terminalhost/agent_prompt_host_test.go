//go:build linux || darwin

package terminalhost

import (
	"context"
	"encoding/json"
	"errors"
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

// questionOf reconciles once and returns the question the session lists.
func questionOf(t *testing.T, m *Manager, id string) *terminalproto.TerminalQuestion {
	t.Helper()
	bellOf(t, m, id)
	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == id {
			return s.Question
		}
	}
	t.Fatal("session not listed")
	return nil
}

// sentKeys replaces send-keys with a recorder: each call is the pane and the
// keys, joined by spaces.
func sentKeys(m *Manager) *[]string {
	var sent []string
	m.sendKeys = func(_ context.Context, pane string, keys []string) error {
		sent = append(sent, pane+" "+strings.Join(keys, " "))
		return nil
	}
	return &sent
}

// The question on an agent's screen is listed with the session, in memory
// only, and goes when the dialog does.
func TestAQuestionOnScreenIsListedUntilItGoes(t *testing.T) {
	m := promptFixture(t)
	id, tty := agentSession(t, m, 57, "claude")
	draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
	q := questionOf(t, m, id)
	if q == nil || q.Question != "Bash command\ngo test ./...\nRun the test suite\nDo you want to proceed?" || len(q.Options) != 3 {
		t.Fatalf("question not listed: %+v", q)
	}
	registry, err := os.ReadFile(m.registryPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(registry), "proceed") || strings.Contains(string(registry), q.Fingerprint) {
		t.Fatal("the question reached the registry")
	}
	changed := m.Changed()
	draw(t, m, id, tty, claudeWorking, "Running tests")
	if q := questionOf(t, m, id); q != nil {
		t.Fatalf("an answered question is still listed: %+v", q)
	}
	select {
	case <-changed:
	default:
		t.Fatal("the question going was not signalled")
	}
}

// An answer is checked against a fresh read of the screen and sends only the
// keys the machine works out, once.
func TestAnswerQuestionSendsTheKeysOnce(t *testing.T) {
	m := promptFixture(t)
	id, tty := agentSession(t, m, 58, "claude")
	draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
	q := questionOf(t, m, id)
	if q == nil {
		t.Fatal("no question")
	}
	sent := sentKeys(m)
	ctx := context.Background()
	if err := m.AnswerQuestion(ctx, id, q.Fingerprint, 3); err != nil {
		t.Fatal(err)
	}
	list, err := m.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatal(err, list)
	}
	pane := list[0].PaneID
	if len(*sent) != 1 || (*sent)[0] != pane+" Down Down Enter" {
		t.Fatalf("sent %q", *sent)
	}
	if err := m.AnswerQuestion(ctx, id, q.Fingerprint, 3); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("a repeat answered %v", err)
	}
	if err := m.AnswerQuestion(ctx, id, q.Fingerprint, 1); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("a second answer to the same question answered %v", err)
	}
	if len(*sent) != 1 {
		t.Fatalf("keys were sent twice: %q", *sent)
	}
	// The agent asks the same question again later: it can be answered again.
	draw(t, m, id, tty, claudeWorking, "Running tests")
	questionOf(t, m, id)
	draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
	questionOf(t, m, id)
	if err := m.AnswerQuestion(ctx, id, q.Fingerprint, 1); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != 2 || (*sent)[1] != pane+" Enter" {
		t.Fatalf("sent %q", *sent)
	}
}

// A screen that no longer shows the question refuses the answer and sends
// nothing, even before the sweep has noticed.
func TestAnswerQuestionRefusesAChangedScreen(t *testing.T) {
	m := promptFixture(t)
	id, tty := agentSession(t, m, 59, "codex")
	draw(t, m, id, tty, codexCommandPrompt, "make migrate")
	q := questionOf(t, m, id)
	if q == nil {
		t.Fatal("no question")
	}
	sent := sentKeys(m)
	ctx := context.Background()
	draw(t, m, id, tty, strings.Replace(codexCommandPrompt, "make migrate", "make drop-db", 1), "make drop-db")
	if err := m.AnswerQuestion(ctx, id, q.Fingerprint, 1); !errors.Is(err, ErrQuestionChanged) {
		t.Fatalf("a changed question answered %v", err)
	}
	draw(t, m, id, tty, codexWorking, "Working")
	if err := m.AnswerQuestion(ctx, id, q.Fingerprint, 1); !errors.Is(err, ErrQuestionChanged) {
		t.Fatalf("a gone question answered %v", err)
	}
	if err := m.AnswerQuestion(ctx, testSessionID(99), q.Fingerprint, 1); !errors.Is(err, ErrSessionMissing) {
		t.Fatalf("an unknown session answered %v", err)
	}
	if len(*sent) != 0 {
		t.Fatalf("keys were sent: %q", *sent)
	}
}

// An agent that asks the same question again after it was answered is asking
// anew: the question is listed again, under a new fingerprint, and can be
// answered again, even when no look saw the screen without it in between.
// A repeat of one answer still sends its keys once.
func TestTheSameQuestionAskedAgainCanBeAnsweredAgain(t *testing.T) {
	m := promptFixture(t)
	id, tty := agentSession(t, m, 60, "claude")
	draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
	first := questionOf(t, m, id)
	if first == nil {
		t.Fatal("no question")
	}
	sent := sentKeys(m)
	ctx := context.Background()
	if err := m.AnswerQuestion(ctx, id, first.Fingerprint, 1); err != nil {
		t.Fatal(err)
	}
	// Still on screen in the second it was answered: the same question.
	if q := questionOf(t, m, id); q == nil || q.Fingerprint != first.Fingerprint {
		t.Fatalf("the answered question changed before the agent drew anything: %+v", q)
	}
	answered := time.Now().Unix()
	for time.Now().Unix() <= answered {
		time.Sleep(20 * time.Millisecond)
	}
	// The agent drew the same dialog again; only its output above changed.
	draw(t, m, id, tty, strings.Replace(claudeCommandPrompt, "tests now", "tests again", 1), "tests again")
	again := questionOf(t, m, id)
	if again == nil || again.Fingerprint == first.Fingerprint {
		t.Fatalf("the question asked again is not listed as a new one: %+v", again)
	}
	if err := m.AnswerQuestion(ctx, id, again.Fingerprint, 2); err != nil {
		t.Fatal(err)
	}
	if err := m.AnswerQuestion(ctx, id, again.Fingerprint, 2); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("a repeat answered %v", err)
	}
	if len(*sent) != 2 || !strings.HasSuffix((*sent)[1], " Down Enter") {
		t.Fatalf("sent %q", *sent)
	}
}

// A pane whose process has gone takes no keys: the answer is refused as a
// changed question and nothing is sent.
func TestAnswerQuestionRefusesADeadPane(t *testing.T) {
	m := promptFixture(t)
	id, tty := agentSession(t, m, 61, "claude")
	draw(t, m, id, tty, claudeCommandPrompt, "Do you want to proceed?")
	q := questionOf(t, m, id)
	if q == nil {
		t.Fatal("no question")
	}
	sent := sentKeys(m)
	m.paneDead = func(context.Context, string) (bool, error) { return true, nil }
	if err := m.AnswerQuestion(context.Background(), id, q.Fingerprint, 1); !errors.Is(err, ErrQuestionChanged) {
		t.Fatalf("a dead pane answered %v", err)
	}
	if len(*sent) != 0 {
		t.Fatalf("keys were sent to a dead pane: %q", *sent)
	}
}
