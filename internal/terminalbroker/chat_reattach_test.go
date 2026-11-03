package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func reattachFixture(t *testing.T) (*Broker, *Peer, int64, string, *switchMachine) {
	t.Helper()
	return switchFixtureCaps(t, func(c *terminalproto.Capabilities) { c.ChatReattach = true })
}

// lostChat starts an OpenCode chat, puts one event in its ring and marks the
// row lost, as a reconcile that no longer finds it would.
func lostChat(t *testing.T, b *Broker, p *Peer, uid int64, agent string) store.TerminalSession {
	t.Helper()
	s := switchChat(t, b, uid, agent)
	b.publishChat(uid, s.ID, terminalproto.ACPEvent{Kind: "user", Text: "earlier message"})
	if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Lost {
		t.Fatalf("row %+v", got)
	}
	return s
}

func reattachSent(m *switchMachine) []*terminalproto.ChatReattach {
	var out []*terminalproto.ChatReattach
	for _, msg := range m.sent() {
		if in, ok := msg.Body.(*terminalproto.ChatReattach); ok {
			out = append(out, in)
		}
	}
	return out
}

func reattachEvents(t *testing.T, b *Broker, uid int64) []string {
	t.Helper()
	events, err := b.st.ListEventsFor(uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range events {
		if e.Action == "chat-reattach" {
			out = append(out, e.Detail)
		}
	}
	return out
}

func noCreatingRows(t *testing.T, b *Broker, uid int64) []store.TerminalSession {
	t.Helper()
	all, err := b.st.TerminalSessionsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		if s.State == terminalproto.Creating {
			t.Fatalf("row left creating: %+v", s)
		}
	}
	return all
}

func TestReattachLostChatBecomesRunningInPlace(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	if err := b.ReattachChat(t.Context(), uid, s.ID); err != nil {
		t.Fatal(err)
	}
	got := row(t, b, uid, s.ID)
	if got.ID != s.ID || got.State != terminalproto.Running || !got.Structured || got.ACPSessionID != switchACPID {
		t.Fatalf("row after reattach %+v", got)
	}
	if all := noCreatingRows(t, b, uid); len(all) != 1 {
		t.Fatalf("rows %+v", all)
	}
	sent := machine.sent()
	if len(sent) != 1 || sent[0].SessionID != s.ID {
		t.Fatalf("sent %+v", sent)
	}
	in, ok := sent[0].Body.(*terminalproto.ChatReattach)
	if !ok || in.Tool != "opencode" || in.Dir != "/tmp" || in.ACPSessionID != switchACPID || in.Title != "Chat" {
		t.Fatalf("sent body %+v", sent[0].Body)
	}
	snapshot, _, _, cancel := b.SubscribeChat(uid, s.ID)
	cancel()
	found := false
	for _, e := range snapshot {
		found = found || e.Kind == "user" && e.Text == "earlier message"
	}
	if !found {
		t.Fatalf("ring lost the earlier transcript: %+v", snapshot)
	}
	if events := reattachEvents(t, b, uid); len(events) != 1 {
		t.Fatalf("audit %+v", events)
	}
}

func TestReattachExitedChat(t *testing.T) {
	b, p, uid, agent, _ := reattachFixture(t)
	s := switchChat(t, b, uid, agent)
	code := 1
	if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, terminalproto.Exited, &code); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, s.ID); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Running || got.ExitCode != nil {
		t.Fatalf("row %+v", got)
	}
}

func TestReattachRefusedForEndedAndRunningAndShell(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	running := switchChat(t, b, uid, agent)
	if err := b.ReattachChat(t.Context(), uid, running.ID); !errors.Is(err, ErrReattachNotClosed) {
		t.Fatalf("running: %v", err)
	}
	ended := switchChat(t, b, uid, agent)
	if _, err := b.End(t.Context(), uid, ended.ID, ID()); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, ended.ID); !errors.Is(err, ErrACPClosed) {
		t.Fatalf("ended: %v", err)
	}
	shell, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "TUI", Cols: 100, Rows: 30, Tool: "opencode", Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.st.ObserveTerminalStateFor(uid, agent, shell.ID, p.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, shell.ID); !errors.Is(err, ErrACPUnsupported) {
		t.Fatalf("shell: %v", err)
	}
	if got := row(t, b, uid, shell.ID); got.State != terminalproto.Lost {
		t.Fatalf("shell row %+v", got)
	}
	if got := row(t, b, uid, ended.ID); got.State != terminalproto.Ended {
		t.Fatalf("ended row %+v", got)
	}
	if sent := reattachSent(machine); len(sent) != 0 {
		t.Fatalf("sent %+v", sent)
	}
}

func TestReattachRefusedWithoutFeatureOrHistory(t *testing.T) {
	for name, adjust := range map[string]func(*terminalproto.Capabilities){
		"feature": func(c *terminalproto.Capabilities) {},
		"history": func(c *terminalproto.Capabilities) {
			c.ChatReattach = true
			c.ACPAgents = &[]terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			b, p, uid, agent, machine := switchFixtureCaps(t, adjust)
			s := lostChat(t, b, p, uid, agent)
			if err := b.ReattachChat(t.Context(), uid, s.ID); !errors.Is(err, ErrReattachUnsupported) {
				t.Fatalf("got %v", err)
			}
			if sent := machine.sent(); len(sent) != 0 {
				t.Fatalf("sent %+v", sent)
			}
			if got := row(t, b, uid, s.ID); got.State != terminalproto.Lost {
				t.Fatalf("row %+v", got)
			}
		})
	}
}

func TestReattachRefusedWithoutACPSessionID(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	machine.mu.Lock()
	machine.noChatID = true
	machine.mu.Unlock()
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, s.ID); !errors.Is(err, ErrReattachNoConversation) {
		t.Fatalf("got %v", err)
	}
	if sent := reattachSent(machine); len(sent) != 0 {
		t.Fatalf("sent %+v", sent)
	}
}

func TestReattachSingleFlightSendsOnce(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	release := make(chan struct{})
	machine.mu.Lock()
	machine.reattach = func(_ *Peer, _ terminalproto.Message, in *terminalproto.ChatReattach) terminalproto.Body {
		<-release
		return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: in.ACPSessionID}
	}
	machine.mu.Unlock()
	const callers = 10
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = b.ReattachChat(t.Context(), uid, s.ID)
		}()
	}
	deadline := time.Now().Add(10 * time.Second)
	for b.reattachJoined(uid, s.ID) < callers-1 {
		if time.Now().After(deadline) {
			t.Fatalf("joined %d callers", b.reattachJoined(uid, s.ID))
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}
	if sent := reattachSent(machine); len(sent) != 1 {
		t.Fatalf("sent %d chat_reattach messages", len(sent))
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Running {
		t.Fatalf("row %+v", got)
	}
	// The in-flight entry is gone: a later call runs again (and is refused
	// because the chat now runs).
	if err := b.ReattachChat(t.Context(), uid, s.ID); !errors.Is(err, ErrReattachNotClosed) {
		t.Fatalf("later call: %v", err)
	}
}

func TestReattachOfflineLeavesRowLostAndNoCreatingRows(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	machine.mu.Lock()
	machine.reattachSendErr = errors.New("connection lost")
	machine.mu.Unlock()
	err := b.ReattachChat(t.Context(), uid, s.ID)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("got %v", err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Lost {
		t.Fatalf("row %+v", got)
	}
	if all := noCreatingRows(t, b, uid); len(all) != 1 {
		t.Fatalf("rows %+v", all)
	}
	// Retryable: once the machine answers, the same row comes back.
	machine.mu.Lock()
	machine.reattachSendErr = nil
	machine.mu.Unlock()
	if err := b.ReattachChat(t.Context(), uid, s.ID); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Running {
		t.Fatalf("row %+v", got)
	}
}

func TestReattachMachineConflictMapsToErrReattachConflict(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	machine.mu.Lock()
	machine.reattach = func(*Peer, terminalproto.Message, *terminalproto.ChatReattach) terminalproto.Body {
		// chatResult(terminalhost.ErrOperationConflict) on the machine.
		return terminalproto.Result{Code: terminalproto.Conflict}
	}
	machine.mu.Unlock()
	err := b.ReattachChat(t.Context(), uid, s.ID)
	if !errors.Is(err, ErrReattachConflict) || err.Error() != "This conversation is open in another session." {
		t.Fatalf("got %v", err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Lost {
		t.Fatalf("row %+v", got)
	}
	noCreatingRows(t, b, uid)
}

func TestReattachMachineRefusalsMap(t *testing.T) {
	cases := []struct {
		name  string
		reply terminalproto.Body
		check func(error) bool
	}{
		{"trust", terminalproto.Result{Code: terminalproto.ACPTrustRequired, Detail: "This folder has agent extensions: x. Start a chat only in a folder you trust."}, func(err error) bool {
			var trust *ACPTrustError
			return errors.As(err, &trust) && trust.Detail != ""
		}},
		{"version", terminalproto.Result{Code: terminalproto.ACPVersion, Detail: "old"}, func(err error) bool { return errors.Is(err, ErrACPVersion) }},
		{"unavailable", terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "That agent could not start the chat in that folder."}, func(err error) bool {
			var unavailable *ACPUnavailableError
			return errors.As(err, &unavailable)
		}},
		{"unavailable without detail", terminalproto.Result{Code: terminalproto.ACPUnavailable}, func(err error) bool { return errors.Is(err, ErrACPUnsupported) }},
		{"not found", terminalproto.Result{Code: terminalproto.NotFound}, func(err error) bool { return errors.Is(err, ErrReattachFailed) }},
		{"unavailable shell", terminalproto.Result{Code: terminalproto.Unavailable}, func(err error) bool { return errors.Is(err, ErrReattachFailed) }},
		{"still creating", terminalproto.SessionChanged{State: terminalproto.Creating}, func(err error) bool { return errors.Is(err, ErrReattachFailed) }},
		{"ended", terminalproto.SessionChanged{State: terminalproto.Ended}, func(err error) bool { return errors.Is(err, ErrReattachFailed) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, p, uid, agent, machine := reattachFixture(t)
			s := lostChat(t, b, p, uid, agent)
			machine.mu.Lock()
			machine.reattach = func(*Peer, terminalproto.Message, *terminalproto.ChatReattach) terminalproto.Body { return tc.reply }
			machine.mu.Unlock()
			if err := b.ReattachChat(t.Context(), uid, s.ID); !tc.check(err) {
				t.Fatalf("got %v", err)
			}
			if got := row(t, b, uid, s.ID); got.State != terminalproto.Lost {
				t.Fatalf("row %+v", got)
			}
			noCreatingRows(t, b, uid)
			if events := reattachEvents(t, b, uid); len(events) != 0 {
				t.Fatalf("audit %+v", events)
			}
		})
	}
}

func TestReattachRecordsReportedACPSessionID(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	machine.mu.Lock()
	machine.reattach = func(*Peer, terminalproto.Message, *terminalproto.ChatReattach) terminalproto.Body {
		// An older reply carries no id: the stored one is kept.
		return terminalproto.SessionChanged{State: terminalproto.Running}
	}
	machine.mu.Unlock()
	if err := b.ReattachChat(t.Context(), uid, s.ID); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Running || got.ACPSessionID != switchACPID {
		t.Fatalf("row %+v", got)
	}
}

func TestReattachOtherAccountRefused(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	other, err := b.st.CreateUser("other@example.com", "Other", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{other.ID, uid + 1000} {
		if err := b.ReattachChat(t.Context(), id, s.ID); err == nil {
			t.Fatalf("account %d reattached the chat", id)
		}
	}
	if sent := reattachSent(machine); len(sent) != 0 {
		t.Fatalf("sent %+v", sent)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Lost {
		t.Fatalf("row %+v", got)
	}
}

func TestReattachSurvivesCancelledRequest(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	ctx, cancel := context.WithCancel(t.Context())
	machine.mu.Lock()
	machine.reattach = func(p *Peer, m terminalproto.Message, in *terminalproto.ChatReattach) terminalproto.Body {
		// The browser goes away while the machine is starting the chat; the
		// answer comes after that.
		cancel()
		go func() {
			time.Sleep(50 * time.Millisecond)
			reply := m
			reply.Body = &terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: in.ACPSessionID}
			_ = p.Receive(reply)
		}()
		return nil
	}
	machine.mu.Unlock()
	if err := b.ReattachChat(ctx, uid, s.ID); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Running {
		t.Fatalf("row %+v", got)
	}
}
