package terminalbroker

import (
	"context"
	"errors"
	"strings"
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
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); err != nil {
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
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Running || got.ExitCode != nil {
		t.Fatalf("row %+v", got)
	}
}

func TestReattachRefusedForEndedAndRunningAndShell(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	running := switchChat(t, b, uid, agent)
	if err := b.ReattachChat(t.Context(), uid, running.ID, false); !errors.Is(err, ErrReattachNotClosed) {
		t.Fatalf("running: %v", err)
	}
	ended := switchChat(t, b, uid, agent)
	if _, err := b.End(t.Context(), uid, ended.ID, ID()); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, ended.ID, false); !errors.Is(err, ErrACPClosed) {
		t.Fatalf("ended: %v", err)
	}
	shell, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "TUI", Cols: 100, Rows: 30, Tool: "opencode", Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.st.ObserveTerminalStateFor(uid, agent, shell.ID, p.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, shell.ID, false); !errors.Is(err, ErrACPUnsupported) {
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

// The real machine answers a person's End of a chat with exited (code 0),
// not ended: the chat must still never be reconnected in place.
func TestReattachRefusedAfterRealEndAnswer(t *testing.T) {
	b, _, uid, agent, machine := reattachFixture(t)
	machine.mu.Lock()
	machine.endExited = true
	machine.mu.Unlock()
	s := switchChat(t, b, uid, agent)
	if _, err := b.End(t.Context(), uid, s.ID, ID()); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Ended {
		t.Fatalf("ended chat stored as %+v", got)
	}
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); !errors.Is(err, ErrACPClosed) {
		t.Fatalf("got %v", err)
	}
	if sent := reattachSent(machine); len(sent) != 0 {
		t.Fatalf("sent %+v", sent)
	}
}

// A chat a person ended before ended chats were recorded as ended is stored
// exited (or lost, when the machine went away meanwhile); its end operation
// still refuses a reattach.
func TestReattachRefusedForClosedRowWithEndOperation(t *testing.T) {
	for _, state := range []terminalproto.SessionState{terminalproto.Exited, terminalproto.Lost} {
		t.Run(string(state), func(t *testing.T) {
			b, p, uid, agent, machine := reattachFixture(t)
			s := switchChat(t, b, uid, agent)
			var code *int
			if state == terminalproto.Exited {
				zero := 0
				code = &zero
			}
			if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, state, code); err != nil {
				t.Fatal(err)
			}
			if _, err := b.st.RequestTerminalEndFor(uid, agent, s.ID, ID()); err != nil {
				t.Fatal(err)
			}
			if err := b.ReattachChat(t.Context(), uid, s.ID, false); !errors.Is(err, ErrACPClosed) {
				t.Fatalf("got %v", err)
			}
			if got := row(t, b, uid, s.ID); got.State != state {
				t.Fatalf("row %+v", got)
			}
			if sent := reattachSent(machine); len(sent) != 0 {
				t.Fatalf("sent %+v", sent)
			}
		})
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
			if err := b.ReattachChat(t.Context(), uid, s.ID, false); !errors.Is(err, ErrReattachUnsupported) {
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
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); !errors.Is(err, ErrReattachNoConversation) {
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
			errs[i] = b.ReattachChat(t.Context(), uid, s.ID, false)
		}()
	}
	deadline := time.Now().Add(10 * time.Second)
	for b.reattachJoined(uid, s.ID, false) < callers-1 {
		if time.Now().After(deadline) {
			t.Fatalf("joined %d callers", b.reattachJoined(uid, s.ID, false))
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
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); !errors.Is(err, ErrReattachNotClosed) {
		t.Fatalf("later call: %v", err)
	}
}

func TestReattachOfflineLeavesRowLostAndNoCreatingRows(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	machine.mu.Lock()
	machine.reattachSendErr = errors.New("connection lost")
	machine.mu.Unlock()
	err := b.ReattachChat(t.Context(), uid, s.ID, false)
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
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); err != nil {
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
	err := b.ReattachChat(t.Context(), uid, s.ID, false)
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
			if err := b.ReattachChat(t.Context(), uid, s.ID, false); !tc.check(err) {
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
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); err != nil {
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
		if err := b.ReattachChat(t.Context(), id, s.ID, false); err == nil {
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
	if err := b.ReattachChat(ctx, uid, s.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Running {
		t.Fatalf("row %+v", got)
	}
}

// A YOLO chat is a YOLO launch: reconnecting it needs the account to allow
// YOLO, checked before the machine is asked.
func TestReattachYoloChatNeedsAccountAllowYolo(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := switchChat(t, b, uid, agent)
	if err := b.st.SetTerminalYoloFor(uid, s.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.st.SetSetting("terminal_allow_yolo", "0"); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); !errors.Is(err, ErrYoloNotAllowed) {
		t.Fatalf("got %v", err)
	}
	if sent := reattachSent(machine); len(sent) != 0 {
		t.Fatalf("sent %+v", sent)
	}
	if got := row(t, b, uid, s.ID); got.State != terminalproto.Lost {
		t.Fatalf("row %+v", got)
	}
	if err := b.st.SetSetting("terminal_allow_yolo", "1"); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); err != nil {
		t.Fatal(err)
	}
	if sent := reattachSent(machine); len(sent) != 1 || !sent[0].Yolo {
		t.Fatalf("sent %+v", sent)
	}
}

// Refusals that a retry cannot fix keep their own errors, not the
// retryable ErrReattachFailed; a chat still starting is its own retryable
// error, not "open in another session".
func TestReattachMachineRefusalsKeepTheirMeaning(t *testing.T) {
	cases := []struct {
		name  string
		reply terminalproto.Body
		want  error
	}{
		{"yolo refused", terminalproto.Result{Code: terminalproto.YoloRefused}, ErrYoloRefused},
		{"capacity", terminalproto.Result{Code: terminalproto.Capacity}, store.ErrTerminalCapacity},
		{"starting", terminalproto.Result{Code: terminalproto.Conflict, Detail: terminalproto.ConflictStarting}, ErrReattachStarting},
		{"owner elsewhere", terminalproto.Result{Code: terminalproto.Conflict}, ErrReattachConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, p, uid, agent, machine := reattachFixture(t)
			s := lostChat(t, b, p, uid, agent)
			machine.mu.Lock()
			machine.reattach = func(*Peer, terminalproto.Message, *terminalproto.ChatReattach) terminalproto.Body { return tc.reply }
			machine.mu.Unlock()
			err := b.ReattachChat(t.Context(), uid, s.ID, false)
			if !errors.Is(err, tc.want) || errors.Is(err, ErrReattachFailed) {
				t.Fatalf("got %v", err)
			}
			if got := row(t, b, uid, s.ID); got.State != terminalproto.Lost {
				t.Fatalf("row %+v", got)
			}
		})
	}
	if ErrReattachStarting.Error() != "Still reconnecting. Try again in a moment." {
		t.Fatal(ErrReattachStarting)
	}
}

func TestReattachRefusedForArchivedChat(t *testing.T) {
	for _, state := range []terminalproto.SessionState{terminalproto.Lost, terminalproto.Exited} {
		t.Run(string(state), func(t *testing.T) {
			b, p, uid, agent, machine := reattachFixture(t)
			s := switchChat(t, b, uid, agent)
			var exit *int
			if state == terminalproto.Exited {
				one := 1
				exit = &one
			}
			if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, state, exit); err != nil {
				t.Fatal(err)
			}
			if err := b.st.ArchiveTerminalFor(uid, s.ID); err != nil {
				t.Fatal(err)
			}
			if err := b.ReattachChat(t.Context(), uid, s.ID, false); !errors.Is(err, ErrReattachArchived) {
				t.Fatalf("archived: %v", err)
			}
			if sent := reattachSent(machine); len(sent) != 0 {
				t.Fatalf("sent %+v", sent)
			}
			if got := row(t, b, uid, s.ID); got.State != state {
				t.Fatalf("row %+v", got)
			}
		})
	}
}

// A reattach without trust sends Trust:false and records trust "false"; one
// with trust sends Trust:true and records trust "true".
func TestReattachTrustIsSentAndAudited(t *testing.T) {
	for _, trust := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "trusted"}[trust], func(t *testing.T) {
			b, p, uid, agent, machine := reattachFixture(t)
			s := lostChat(t, b, p, uid, agent)
			if err := b.ReattachChat(t.Context(), uid, s.ID, trust); err != nil {
				t.Fatal(err)
			}
			sent := reattachSent(machine)
			if len(sent) != 1 || sent[0].Trust != trust {
				t.Fatalf("sent %+v, want Trust %v", sent, trust)
			}
			events := reattachEvents(t, b, uid)
			want := map[bool]string{false: `"trust":"false"`, true: `"trust":"true"`}[trust]
			if len(events) != 1 || !strings.Contains(events[0], want) {
				t.Fatalf("audit %+v, want %s", events, want)
			}
		})
	}
}

// Trust is never remembered: after a trusted reattach, the next reattach of
// the same chat without trust sends Trust:false.
func TestReattachTrustIsNotRemembered(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	if err := b.ReattachChat(t.Context(), uid, s.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.ReattachChat(t.Context(), uid, s.ID, false); err != nil {
		t.Fatal(err)
	}
	sent := reattachSent(machine)
	if len(sent) != 2 || !sent[0].Trust || sent[1].Trust {
		t.Fatalf("sent %+v", sent)
	}
}

// A trusting call never joins a non-trusting call in flight (or the reverse):
// each sends its own message with its own Trust.
func TestReattachSingleFlightKeepsTrustApart(t *testing.T) {
	b, p, uid, agent, machine := reattachFixture(t)
	s := lostChat(t, b, p, uid, agent)
	release := make(chan struct{})
	machine.mu.Lock()
	machine.reattach = func(_ *Peer, _ terminalproto.Message, in *terminalproto.ChatReattach) terminalproto.Body {
		<-release
		return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: in.ACPSessionID}
	}
	machine.mu.Unlock()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, trust := range []bool{false, true} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = b.ReattachChat(t.Context(), uid, s.ID, trust)
		}()
	}
	deadline := time.Now().Add(10 * time.Second)
	for len(reattachSent(machine)) < 2 {
		if time.Now().After(deadline) {
			close(release)
			wg.Wait()
			t.Fatalf("sent %d chat_reattach messages, want 2 (joined untrusted %d, trusted %d)",
				len(reattachSent(machine)), b.reattachJoined(uid, s.ID, false), b.reattachJoined(uid, s.ID, true))
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	sent := reattachSent(machine)
	trusts := map[bool]int{}
	for _, in := range sent {
		trusts[in.Trust]++
	}
	if len(sent) != 2 || trusts[true] != 1 || trusts[false] != 1 {
		t.Fatalf("sent %+v", sent)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}
}

// An archived chat answers archived even when a person also ended it (end and
// archive in one tab, Reconnect from a stale other tab): its transcript is
// gone, so the ended answer's Continue with history must not be offered.
func TestReattachArchivedWinsOverEnded(t *testing.T) {
	for _, name := range []string{"lost with end operation", "ended"} {
		t.Run(name, func(t *testing.T) {
			b, p, uid, agent, machine := reattachFixture(t)
			s := switchChat(t, b, uid, agent)
			if name == "ended" {
				if _, err := b.End(t.Context(), uid, s.ID, ID()); err != nil {
					t.Fatal(err)
				}
				if err := b.ReattachChat(t.Context(), uid, s.ID, false); !errors.Is(err, ErrACPClosed) {
					t.Fatalf("ended before archive: %v", err)
				}
			} else {
				if err := b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, terminalproto.Lost, nil); err != nil {
					t.Fatal(err)
				}
				if _, err := b.st.RequestTerminalEndFor(uid, agent, s.ID, ID()); err != nil {
					t.Fatal(err)
				}
			}
			before := len(reattachSent(machine))
			if err := b.st.ArchiveTerminalFor(uid, s.ID); err != nil {
				t.Fatal(err)
			}
			if err := b.ReattachChat(t.Context(), uid, s.ID, true); !errors.Is(err, ErrReattachArchived) {
				t.Fatalf("got %v, want ErrReattachArchived", err)
			}
			if sent := reattachSent(machine); len(sent) != before {
				t.Fatalf("sent %+v", sent)
			}
		})
	}
}
