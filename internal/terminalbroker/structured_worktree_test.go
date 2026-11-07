package terminalbroker

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestStructuredWorktreeIsNotDispatchedToAnOldConnector(t *testing.T) {
	calls := 0
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.Worktree = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		switch m.Body.(type) {
		case terminalproto.Create, *terminalproto.Create:
			calls++
		}
		return nil, true
	})
	_, _, err := b.CreateSession(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Isolated Chat", Tool: "opencode", Structured: true, Cols: 80, Rows: 24, Dir: "/home/test/repo", Project: "project", Worktree: "0123456789abcdef0123456789abcdef"})
	if !errors.Is(err, ErrACPWorktreeUnsupported) {
		t.Fatalf("legacy connector refusal: %v", err)
	}
	if calls != 0 {
		t.Fatal("unsupported create reached the connector")
	}
	sessions, err := b.st.TerminalSessionsFor(uid)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("unsupported create reserved quota: %d %v", len(sessions), err)
	}
}
