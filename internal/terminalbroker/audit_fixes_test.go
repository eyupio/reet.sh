package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// slowMachineFixture is a machine that takes a create and answers it only when
// the test says so, and answers a reconcile (with no sessions) only while
// reconcileAnswers reports true.
func slowMachineFixture(t *testing.T, synchronize bool, reconcileAnswers func() bool) (*Broker, *Peer, int64, string, chan terminalproto.Message) {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "fixture", "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "test", HealthEnabled: true, TerminalEnabled: true, TerminalControl: true, Readiness: terminalproto.Ready}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	b := New(st)
	t.Cleanup(b.Close)
	creates := make(chan terminalproto.Message, 4)
	var p *Peer
	var mu sync.Mutex
	p, err = b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		mu.Lock()
		defer mu.Unlock()
		switch m.Body.(type) {
		case terminalproto.Reconcile:
			if !reconcileAnswers() {
				return nil // machine busy: no answer
			}
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Create:
			creates <- m // still fetching: the answer comes later
			return nil
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		return p.Receive(wire(t, reply))
	})
	if err != nil {
		t.Fatal(err)
	}
	if synchronize {
		if err = p.Synchronize(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	return b, p, uid, a.ID, creates
}

// wire passes a message through the protocol's encoding, as a machine's does.
func wire(t *testing.T, m terminalproto.Message) terminalproto.Message {
	t.Helper()
	data, err := terminalproto.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := terminalproto.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func answer(t *testing.T, sent terminalproto.Message, body terminalproto.Body) terminalproto.Message {
	t.Helper()
	sent.Body = body
	return wire(t, sent)
}

func TestReplayWaitsForACreateStillRunningOnTheMachine(t *testing.T) {
	b, p, uid, agent, creates := slowMachineFixture(t, true, func() bool { return true })
	request := ID()
	in := terminalproto.Create{Title: "Build", Cols: 80, Rows: 24}
	first, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := b.Create(first, uid, agent, request, in); done <- err }()
	sent := <-creates
	cancel() // the browser gave up waiting
	if err := <-done; !errors.Is(err, ErrOffline) {
		t.Fatalf("first create = %v", err)
	}
	type outcome struct {
		s       store.TerminalSession
		learned bool
		err     error
	}
	replayed := make(chan outcome, 1)
	go func() {
		s, learned, err := b.CreateSession(t.Context(), uid, agent, request, in)
		replayed <- outcome{s, learned, err}
	}()
	select {
	case got := <-replayed:
		t.Fatalf("the replay did not wait for the pending create: %v %v", got.s.State, got.err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := p.Receive(answer(t, sent, terminalproto.SessionChanged{State: terminalproto.Running})); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-replayed:
		if got.err != nil || got.s.State != terminalproto.Running || !got.learned {
			t.Fatalf("replay = %v learned=%v err=%v", got.s.State, got.learned, got.err)
		}
		row, err := b.st.TerminalSessionFor(uid, got.s.ID)
		if err != nil || row.State != terminalproto.Running {
			t.Fatalf("row = %v %v", row.State, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the replay never heard the late answer")
	}
}

func TestSynchronizeDoesNotLoseACreateStillRunningOnTheMachine(t *testing.T) {
	b, p, uid, agent, creates := slowMachineFixture(t, true, func() bool { return true })
	request := ID()
	first, cancel := context.WithCancel(t.Context())
	done := make(chan store.TerminalSession, 1)
	go func() {
		s, _ := b.Create(first, uid, agent, request, terminalproto.Create{Title: "Build", Cols: 80, Rows: 24})
		done <- s
	}()
	sent := <-creates
	cancel()
	s := <-done
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if row, _ := b.st.TerminalSessionFor(uid, s.ID); row.State != terminalproto.Creating {
		t.Fatalf("a reconcile during the create left the row %v", row.State)
	}
	if err := p.Receive(answer(t, sent, terminalproto.SessionChanged{State: terminalproto.Running})); err != nil {
		t.Fatal(err)
	}
	p.settleObservations(2 * time.Second)
	if row, _ := b.st.TerminalSessionFor(uid, s.ID); row.State != terminalproto.Running {
		t.Fatalf("row after the machine reported running = %v", row.State)
	}
	// With the answer applied nothing is pending, so a missing session is lost again.
	if p.pendingCreateOf(s.ID) != nil {
		t.Fatal("the answered create is still pending")
	}
}

func TestLateCreateRefusalLetsReconcileSettleTheRow(t *testing.T) {
	b, p, uid, agent, creates := slowMachineFixture(t, true, func() bool { return true })
	request := ID()
	first, cancel := context.WithCancel(t.Context())
	done := make(chan store.TerminalSession, 1)
	go func() {
		s, _ := b.Create(first, uid, agent, request, terminalproto.Create{Title: "Build", Cols: 80, Rows: 24})
		done <- s
	}()
	sent := <-creates
	cancel()
	s := <-done
	if err := p.Receive(answer(t, sent, terminalproto.Result{Code: terminalproto.LaunchFailed})); err != nil {
		t.Fatal(err)
	}
	if err := p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if row, _ := b.st.TerminalSessionFor(uid, s.ID); row.State != terminalproto.Lost {
		t.Fatalf("a refused create stayed %v", row.State)
	}
}

func TestFailedSynchronizeKeepsAReadyPeerReady(t *testing.T) {
	answers := true
	var mu sync.Mutex
	b, p, uid, agent, _ := slowMachineFixture(t, true, func() bool { mu.Lock(); defer mu.Unlock(); return answers })
	old := callTimeout
	callTimeout = 100 * time.Millisecond
	defer func() { callTimeout = old }()
	mu.Lock()
	answers = false
	mu.Unlock()
	if err := p.Synchronize(t.Context()); err == nil {
		t.Fatal("an unanswered reconcile succeeded")
	}
	if !b.Ready(uid, agent) {
		t.Fatal("a failed reconcile left the machine reconnecting")
	}
	select {
	case <-p.Done():
		t.Fatal("a failed reconcile closed a usable connection")
	default:
	}
}

func TestFailedFirstSynchronizeClosesThePeer(t *testing.T) {
	old := callTimeout
	callTimeout = 100 * time.Millisecond
	defer func() { callTimeout = old }()
	b, p, uid, agent, _ := slowMachineFixture(t, false, func() bool { return false })
	if err := p.Synchronize(t.Context()); err == nil {
		t.Fatal("an unanswered reconcile succeeded")
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("a connection that never synchronized stayed open and not ready")
	}
	if b.Ready(uid, agent) {
		t.Fatal("ready without a reconcile")
	}
}

func TestRemoveFromHistoryOnCloseRemovesTheTranscript(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	if err := b.PromptChat(t.Context(), uid, s.ID, "a private prompt", nil); err != nil {
		t.Fatal(err)
	}
	b.chatLog(uid, s.ID).flushLater()
	if _, err := b.End(t.Context(), uid, s.ID, ID(), true); err != nil {
		t.Fatal(err)
	}
	archived, err := b.st.TerminalArchivedFor(uid, s.ID)
	if err != nil || !archived {
		t.Fatalf("archived = %v %v", archived, err)
	}
	if rows, _ := b.st.TerminalChatEventsFor(uid, s.ID); len(rows) != 0 {
		t.Fatalf("%d transcript rows kept for a removed session", len(rows))
	}
	if b.chatExists(uid, s.ID) {
		t.Fatal("the removed session's chat is still in memory")
	}
}

func TestTicketsAreReplacedAndCappedPerAccount(t *testing.T) {
	b, _, uid, agent := fixture(t)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Cols: 80, Rows: 24}
	first, err := b.Ticket(uid, "login", s.ID, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Ticket(uid, "login", s.ID, opts)
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	held := len(b.tickets)
	b.mu.Unlock()
	if held != 1 {
		t.Fatalf("%d tickets outstanding for one login and session", held)
	}
	if _, err := b.consume(first, uid, "login"); !errors.Is(err, ErrTicket) {
		t.Fatalf("a replaced ticket was accepted: %v", err)
	}
	if _, err := b.consume(second, uid, "login"); err != nil {
		t.Fatalf("the newest ticket was refused: %v", err)
	}
	for i := range maxAccountTickets {
		if _, err := b.Ticket(uid, "login-"+string(rune('a'+i)), s.ID, opts); err != nil {
			t.Fatalf("ticket %d: %v", i, err)
		}
	}
	if _, err := b.Ticket(uid, "one-more", s.ID, opts); err == nil {
		t.Fatal("one account went over its share of tickets")
	}
	b.mu.Lock()
	held = len(b.tickets)
	b.mu.Unlock()
	if held != maxAccountTickets {
		t.Fatalf("%d tickets outstanding", held)
	}
}

func TestPreviewSlotsAreSharedBetweenAccounts(t *testing.T) {
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	for range previewShare {
		release, err := acquirePreview(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	short, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if release, err := acquirePreview(short, 1); err == nil {
		release()
		t.Fatal("one account took more than its share of preview slots")
	}
	if len(previewReads) != previewShare {
		t.Fatalf("%d shared slots held", len(previewReads))
	}
	release, err := acquirePreview(t.Context(), 2)
	if err != nil {
		t.Fatal("another account could not read a preview:", err)
	}
	release()
	for _, release := range releases {
		release()
	}
	releases = nil
	previewAccounts.Lock()
	left := len(previewAccounts.slots)
	previewAccounts.Unlock()
	if left != 0 || len(previewReads) != 0 {
		t.Fatalf("slots left behind: %d accounts, %d reads", left, len(previewReads))
	}
}

func TestLaunchProgressNeedsAPendingRequestAndIsBounded(t *testing.T) {
	b, p, uid, agent, creates := slowMachineFixture(t, true, func() bool { return true })
	note := func(request string) {
		t.Helper()
		m := terminalproto.Message{Version: terminalproto.Version, Generation: p.Generation, RequestID: request, Body: terminalproto.LaunchProgress{Stage: terminalproto.StageWorktree}}
		if err := p.Receive(wire(t, m)); err != nil {
			t.Fatal(err)
		}
	}
	stray := ID()
	note(stray)
	if got := b.LaunchProgress(uid, agent, stray); len(got) != 0 {
		t.Fatalf("a stage for a request nobody made was kept: %+v", got)
	}
	request := ID()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = b.Create(ctx, uid, agent, request, terminalproto.Create{Title: "Build", Cols: 80, Rows: 24})
	}()
	<-creates
	for range progressSteps + 20 {
		note(request)
	}
	if got := len(b.LaunchProgress(uid, agent, request)); got != progressSteps {
		t.Fatalf("%d stages kept for one create", got)
	}
	cancel()
	<-done
}

func TestOutputQueueCountsBytesNotFrames(t *testing.T) {
	q := newOutputQueue()
	for i := range 64 {
		if !q.push([]byte{'x'}) {
			t.Fatalf("tiny frame %d closed the attachment", i)
		}
	}
	for range 64 {
		q.took(<-q.frames)
	}
	full := make([]byte, terminalproto.MaxBinaryBytes)
	for i := range terminalproto.MaxQueuedOutputBytes / terminalproto.MaxBinaryBytes {
		if !q.push(full) {
			t.Fatalf("frame %d refused inside the byte budget", i)
		}
	}
	if q.push([]byte{'x'}) {
		t.Fatal("output over the byte budget was queued")
	}
	q = newOutputQueue()
	for range maxQueuedOutputFrames {
		if !q.push([]byte{'x'}) {
			t.Fatal("a frame inside both bounds was refused")
		}
	}
	if q.push([]byte{'x'}) {
		t.Fatal("the frame count is unbounded")
	}
}
