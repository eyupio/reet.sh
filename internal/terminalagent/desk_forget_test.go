//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// deskCall sends one message on the connection and returns its answer.
func deskCall(t *testing.T, handle func(context.Context, terminalproto.Message) error, got replies, request, session string, body terminalproto.Body) terminalproto.Body {
	t.Helper()
	msg := terminalproto.Message{Version: terminalproto.Version, Generation: 1, RequestID: request, SessionID: session, Body: body}
	if err := msg.Validate(); err != nil {
		t.Fatal("not a valid message", err)
	}
	if err := handle(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(30 * time.Second)
	for {
		select {
		case reply := <-got:
			if reply.RequestID == request {
				return reply.Body
			}
		case <-deadline:
			t.Fatalf("no answer to %s", request)
		}
	}
}

// An end with ForgetDesk on a desk-shell-v1 connection removes the history
// file of the desk pane the session was created for; a plain end keeps it.
func TestEndForgetDeskRemovesPaneHistory(t *testing.T) {
	base, err := os.MkdirTemp("/tmp", "reet-desk-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if base, err = filepath.EvalSymlinks(base); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	if err := os.MkdirAll(filepath.Join(home, ".reet", "desks", "history"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	handle, got := agentWithTmux(t, home, terminalproto.ShellDirFeature+","+terminalproto.DeskShellFeature, nil)

	panes := map[string]string{"forget": "pane-forget-0123456789", "keep": "pane-keep-00123456789"}
	sessions := map[string]string{"forget": "session-forget-0123456", "keep": "session-keep-000123456"}
	for _, which := range []string{"forget", "keep"} {
		body := deskCall(t, handle, got, "request-create-"+which+"-0001", sessions[which], &terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24, Desk: terminalproto.DeskRef{Pane: panes[which]}})
		if changed, ok := body.(terminalproto.SessionChanged); !ok || changed.State != terminalproto.Running {
			t.Fatalf("create %s answered %#v", which, body)
		}
		// Whatever shell the test machine has, the pane's history file is
		// where the wrapper would keep it.
		if err := os.WriteFile(filepath.Join(home, ".reet", "desks", "history", panes[which]+".bash"), []byte("make\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, which := range []string{"forget", "keep"} {
		body := deskCall(t, handle, got, "request-end-"+which+"-00001", sessions[which], &terminalproto.End{ForgetDesk: which == "forget"})
		if _, ok := body.(terminalproto.SessionChanged); !ok {
			t.Fatalf("end %s answered %#v", which, body)
		}
	}
	_, err = os.Stat(filepath.Join(home, ".reet", "desks", "history", panes["forget"]+".bash"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the forgotten pane's history is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".reet", "desks", "history", panes["keep"]+".bash")); err != nil {
		t.Fatalf("a plain end removed the pane's history: %v", err)
	}
}
