package store

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalRemoveOnCloseWaitsForHostAndSurvivesRestart(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep", true: "remove"}[remove], func(t *testing.T) {
			s, uid, agent, caps := terminalFixture(t)
			session := createTerminal(t, s, uid, agent, caps)
			request := syncID()
			if err := s.ObserveTerminalStateFor(uid, agent.ID, session.ID, caps.Generation, terminalproto.Running, nil); err != nil {
				t.Fatal(err)
			}
			pending, err := s.RequestTerminalEndFor(uid, agent.ID, session.ID, request, remove)
			if err != nil || pending.State != terminalproto.EndRequested || pending.RemoveHistory != remove {
				t.Fatal(pending, err)
			}
			rows, err := s.TerminalSessionsFor(uid)
			if err != nil || len(rows) != 1 {
				t.Fatal("pending hidden", rows, err)
			}
			reopened, err := Open(s.dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			pending, err = reopened.TerminalSessionFor(uid, session.ID)
			if err != nil || pending.RemoveHistory != remove {
				t.Fatal("lost decision", pending, err)
			}
			if _, err = reopened.RequestTerminalEndFor(uid, agent.ID, session.ID, request, !remove); !errors.Is(err, ErrTerminalConflict) {
				t.Fatal("changed retry", err)
			}
			if err = reopened.ObserveTerminalStateFor(uid, agent.ID, session.ID, caps.Generation, terminalproto.Ended, nil); err != nil {
				t.Fatal(err)
			}
			rows, err = reopened.TerminalSessionsFor(uid)
			want := 1
			if remove {
				want = 0
			}
			if err != nil || len(rows) != want {
				t.Fatal("closed visibility", rows, err)
			}
			if _, err = reopened.RequestTerminalEndFor(uid, agent.ID, session.ID, request, remove); err != nil {
				t.Fatal("immutable receipt lost", err)
			}
			retained, err := reopened.TerminalSessionFor(uid, session.ID)
			if err != nil || retained.State != terminalproto.Ended {
				t.Fatal("record deleted", retained, err)
			}
		})
	}
}

// A close that asked for the history to be removed does not archive a session
// still holding a worktree: the controller removes the worktree first, so its
// own archive follows.
func TestTerminalRemoveOnCloseKeepsASessionWithAWorktree(t *testing.T) {
	s, uid, agent, caps := terminalFixture(t)
	session, _, err := s.ReserveTerminalCreateFor(uid, agent.ID, syncID(), caps.Generation, worktreeCreateIn())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ObserveTerminalStateFor(uid, agent.ID, session.ID, caps.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	path := "/home/u/.reet/worktrees/demo/site/" + storeRun
	if err := s.RecordTerminalWorktreeFor(uid, agent.ID, session.ID, path, "site"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestTerminalEndFor(uid, agent.ID, session.ID, syncID(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.ObserveTerminalStateFor(uid, agent.ID, session.ID, caps.Generation, terminalproto.Ended, nil); err != nil {
		t.Fatal(err)
	}
	if archived, err := s.TerminalArchivedFor(uid, session.ID); err != nil || archived {
		t.Fatal("archived while its worktree was still recorded", archived, err)
	}
	if rows, err := s.TerminalSessionsFor(uid); err != nil || len(rows) != 1 {
		t.Fatal("a session with a worktree left history", rows, err)
	}
	if err := s.ArchiveTerminalFor(uid, session.ID); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.TerminalSessionsFor(uid); err != nil || len(rows) != 0 {
		t.Fatal("archive after the worktree removal", rows, err)
	}
}
