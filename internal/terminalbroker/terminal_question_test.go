package terminalbroker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// questionMachine is a connected machine whose answer to a terminal_answer
// the test chooses; answers records each one it was sent.
type questionMachine struct {
	b       *Broker
	p       *Peer
	uid     int64
	agent   string
	mu      sync.Mutex
	code    terminalproto.ResultCode
	answers []terminalproto.TerminalAnswer
	// hold, when set, keeps the machine from answering until it is closed.
	hold chan struct{}
}

func questionFixture(t *testing.T, supports bool) *questionMachine {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "fixture", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, Attention: true, TerminalQuestion: supports, Readiness: terminalproto.Ready}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	m := &questionMachine{b: New(st), uid: uid, agent: a.ID, code: terminalproto.OK}
	t.Cleanup(m.b.Close)
	m.p, err = m.b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, out terminalproto.Message) error {
		data, err := terminalproto.Encode(out)
		if err != nil {
			return err
		}
		sent, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		reply := sent
		switch op := sent.Body.(type) {
		case *terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case *terminalproto.Create:
			reply.Body = terminalproto.SessionChanged{State: terminalproto.Running}
		case *terminalproto.TerminalAnswer:
			m.mu.Lock()
			m.answers = append(m.answers, *op)
			code, hold := m.code, m.hold
			m.mu.Unlock()
			if hold != nil {
				go func() {
					<-hold
					reply.Body = terminalproto.Result{Code: code}
					_ = m.deliver(reply)
				}()
				return nil
			}
			reply.Body = terminalproto.Result{Code: code}
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		return m.deliver(reply)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return m
}

// deliver hands the controller a machine's message as it would arrive: encoded
// and decoded.
func (m *questionMachine) deliver(msg terminalproto.Message) error {
	data, err := terminalproto.Encode(msg)
	if err != nil {
		return err
	}
	decoded, err := terminalproto.Decode(data)
	if err != nil {
		return err
	}
	return m.p.Receive(decoded)
}

func (m *questionMachine) session(t *testing.T) string {
	t.Helper()
	s, err := m.b.Create(t.Context(), m.uid, m.agent, ID(), terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

// report sends a session change from the machine, with q as its question,
// and waits until the controller has handled it.
func (m *questionMachine) report(t *testing.T, session string, q *terminalproto.TerminalQuestion) {
	t.Helper()
	msg := terminalproto.Message{Version: terminalproto.Version, RequestID: ID(), SessionID: session, Generation: m.p.Generation,
		Body: terminalproto.SessionChanged{State: terminalproto.Running, TerminalQuestion: q}}
	if err := m.deliver(msg); err != nil {
		t.Fatal(err)
	}
	m.p.settleObservations(5e9)
}

func (m *questionMachine) listed(t *testing.T, session string) *terminalproto.TerminalQuestion {
	t.Helper()
	row, err := m.b.st.TerminalSessionFor(m.uid, session)
	if err != nil {
		t.Fatal(err)
	}
	m.b.ApplyTerminalQuestion(m.uid, &row)
	return row.TerminalQuestion
}

func (m *questionMachine) sent() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.answers)
}

const secretQuestion = "Bash command\nSUBJECT_TEXT_NEVER_STORED_97531\nDo you want to run QUESTION_TEXT_NEVER_STORED_24680?"

func waitingQuestion(fingerprint string) *terminalproto.TerminalQuestion {
	return &terminalproto.TerminalQuestion{Question: secretQuestion, Truncated: true, Fingerprint: fingerprint,
		Options: []terminalproto.TerminalOption{{Index: 1, Label: "Yes", Selected: true}, {Index: 2, Label: "OPTION_TEXT_NEVER_STORED_13579"}}}
}

// databaseHolds reports whether any file of the store's database holds text.
func databaseHolds(t *testing.T, st *store.Store, text string) bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(st.Path(), "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err == nil && strings.Contains(string(data), text) {
			return true
		}
	}
	return false
}

// The question a machine reports is kept by the controller in memory only:
// it is listed with the session, never written to the database, and goes
// when the machine says it went or the connection ends.
func TestTerminalQuestionIsKeptInMemoryOnly(t *testing.T) {
	m := questionFixture(t, true)
	session := m.session(t)
	fp := strings.Repeat("a", 64)
	m.report(t, session, waitingQuestion(fp))
	q := m.listed(t, session)
	if q == nil || q.Question != secretQuestion || !q.Truncated || q.Fingerprint != fp || len(q.Options) != 2 {
		t.Fatalf("the question is not listed: %+v", q)
	}
	for _, text := range []string{"SUBJECT_TEXT_NEVER_STORED_97531", "QUESTION_TEXT_NEVER_STORED_24680", "OPTION_TEXT_NEVER_STORED_13579", fp} {
		if databaseHolds(t, m.b.st, text) {
			t.Fatalf("the database holds %q", text)
		}
	}
	m.report(t, session, nil)
	if q := m.listed(t, session); q != nil {
		t.Fatalf("a question that went is still listed: %+v", q)
	}
	m.report(t, session, waitingQuestion(fp))
	m.p.Close()
	if q := m.listed(t, session); q != nil {
		t.Fatalf("a question outlived its connection: %+v", q)
	}
}

// An answer reaches the machine once, with the fingerprint and option given;
// a repeat after it succeeded is already answered and sends nothing.
func TestAnswerTerminalQuestionOnce(t *testing.T) {
	m := questionFixture(t, true)
	session := m.session(t)
	fp := strings.Repeat("b", 64)
	m.report(t, session, waitingQuestion(fp))
	if err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, fp, 2); err != nil {
		t.Fatal(err)
	}
	if m.sent() != 1 || m.answers[0] != (terminalproto.TerminalAnswer{Fingerprint: fp, Option: 2}) {
		t.Fatalf("sent %+v", m.answers)
	}
	if q := m.listed(t, session); q != nil {
		t.Fatalf("an answered question is still listed: %+v", q)
	}
	if err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, fp, 2); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("a repeat answered %v", err)
	}
	if m.sent() != 1 {
		t.Fatalf("a repeat reached the machine: %+v", m.answers)
	}
	// The machine knows better after a reconnect: its own refusal is passed on.
	m.mu.Lock()
	m.code = terminalproto.AlreadyAnswered
	m.mu.Unlock()
	if err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, strings.Repeat("c", 64), 1); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("the machine's already answered came back as %v", err)
	}
}

func TestAnswerTerminalQuestionRefusals(t *testing.T) {
	m := questionFixture(t, true)
	session := m.session(t)
	fp := strings.Repeat("d", 64)
	m.report(t, session, waitingQuestion(fp))
	m.mu.Lock()
	m.code = terminalproto.QuestionChanged
	m.mu.Unlock()
	err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, fp, 1)
	if !errors.Is(err, ErrQuestionChanged) || err.Error() != "The question on the terminal changed. Look at it again before answering." {
		t.Fatalf("a changed question answered %v", err)
	}
	if err := m.b.AnswerTerminalQuestion(t.Context(), m.uid+1000, session, fp, 1); !errors.Is(err, store.ErrRemoteNotFound) {
		t.Fatalf("another account answered %v", err)
	}
	for _, bad := range []struct {
		fp     string
		option int
	}{{"nope", 1}, {fp, 0}, {fp, 10}} {
		if err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, bad.fp, bad.option); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%+v answered %v", bad, err)
		}
	}
	if m.sent() != 1 {
		t.Fatalf("refused answers reached the machine: %+v", m.answers)
	}
}

// One answer at a time per session: a second while the first is on its way
// is refused without reaching the machine.
func TestAnswerTerminalQuestionOneAtATime(t *testing.T) {
	m := questionFixture(t, true)
	session := m.session(t)
	fp := strings.Repeat("e", 64)
	m.report(t, session, waitingQuestion(fp))
	m.hold = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- m.b.AnswerTerminalQuestion(context.Background(), m.uid, session, fp, 1) }()
	for m.sent() == 0 {
		select {
		case err := <-done:
			t.Fatalf("the first answer returned early: %v", err)
		default:
		}
	}
	if err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, fp, 2); !errors.Is(err, ErrAnswerBusy) {
		t.Fatalf("a second answer in flight answered %v", err)
	}
	close(m.hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if m.sent() != 1 {
		t.Fatalf("sent %+v", m.answers)
	}
}

// A machine without the feature is told apart, and nothing is sent to it.
func TestAnswerTerminalQuestionNeedsTheFeature(t *testing.T) {
	m := questionFixture(t, false)
	session := m.session(t)
	err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, strings.Repeat("f", 64), 1)
	if !errors.Is(err, ErrTerminalQuestionUnsupported) || err.Error() != "Update reet on this machine to answer from here." {
		t.Fatalf("answered %v", err)
	}
	if m.sent() != 0 {
		t.Fatal("an answer reached a machine without the feature")
	}
}

// A question the agent asks again after an answer comes from the machine
// under a new fingerprint: it is listed again and its answer reaches the
// machine.
func TestAnswerTerminalQuestionAskedAgain(t *testing.T) {
	m := questionFixture(t, true)
	session := m.session(t)
	first, again := strings.Repeat("1", 64), strings.Repeat("2", 64)
	m.report(t, session, waitingQuestion(first))
	if err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, first, 1); err != nil {
		t.Fatal(err)
	}
	m.report(t, session, waitingQuestion(again))
	if q := m.listed(t, session); q == nil || q.Fingerprint != again {
		t.Fatalf("the question asked again is not listed: %+v", q)
	}
	if err := m.b.AnswerTerminalQuestion(t.Context(), m.uid, session, again, 1); err != nil {
		t.Fatal(err)
	}
	if m.sent() != 2 {
		t.Fatalf("sent %+v", m.answers)
	}
}
