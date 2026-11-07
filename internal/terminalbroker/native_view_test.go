package terminalbroker

import (
	"errors"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"testing"
)

func TestNativeViewTicketsKeepAccountAndPresentationBoundaries(t *testing.T) {
	b, _, uid, agent := fixture(t)
	session, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Existing terminal", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.NativeViewInfo(t.Context(), uid+1, session.ID); !errors.Is(err, ErrDenied) {
		t.Fatal("another account read a native session", err)
	}
	if _, err := b.Ticket(uid, "login", session.ID, Options{Cols: 80, Rows: 24, NativeView: true}); !errors.Is(err, ErrDenied) {
		t.Fatal("a native-view ticket attached a plain terminal", err)
	}
	if _, err := b.Ticket(uid, "login", session.ID, Options{Cols: 80, Rows: 24}); err != nil {
		t.Fatal("ordinary terminal attachment regressed", err)
	}
}
func TestNativeViewRequiresAnAdvertisedLiveChat(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	session, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Existing chat", Cols: 80, Rows: 24, Tool: "opencode", Structured: true})
	if err != nil {
		t.Fatal(err)
	}
	info, err := b.NativeViewInfo(t.Context(), uid, session.ID)
	if err != nil || info.Available {
		t.Fatal("legacy connector offered a native view", info, err)
	}
	for _, native := range []bool{false, true} {
		if _, err := b.Ticket(uid, "login", session.ID, Options{Cols: 80, Rows: 24, NativeView: native}); !errors.Is(err, ErrDenied) {
			t.Fatal("chat accepted a terminal ticket without the native feature", err)
		}
	}
}

func TestNativeViewInfoAndTicketUseTheExistingSession(t *testing.T) {
	calls := 0
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.NativeView = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		switch m.Body.(type) {
		case terminalproto.NativeView, *terminalproto.NativeView:
			calls++
			return terminalproto.NativeViewResult{Available: true}, true
		}
		return nil, true
	})
	session, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Shared view", Cols: 80, Rows: 24, Tool: "opencode", Structured: true})
	if err != nil {
		t.Fatal(err)
	}
	info, err := b.NativeViewInfo(t.Context(), uid, session.ID)
	if err != nil || !info.Available || calls != 1 {
		t.Fatalf("runtime availability was not read from this chat: %+v %v calls=%d", info, err, calls)
	}
	ticket, err := b.Ticket(uid, "login", session.ID, Options{Cols: 80, Rows: 24, NativeView: true})
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := b.consume(ticket, uid, "login")
	if err != nil || consumed.session != session.ID || !consumed.opts.NativeView {
		t.Fatal("native attachment replaced or escaped the session", err)
	}
	unchanged, err := b.st.TerminalSessionFor(uid, session.ID)
	if err != nil || !unchanged.Structured || unchanged.ACPSessionID != session.ACPSessionID {
		t.Fatal("view selection changed the worker identity", err)
	}
}
