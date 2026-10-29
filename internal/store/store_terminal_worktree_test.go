package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const storeRun = "0123456789abcdef01234567"

func worktreeCreateIn() terminalproto.Create {
	return terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: "/home/u/site", Worktree: storeRun, Project: "demo"}
}

func TestTerminalSessionRecordsWhereItsWorktreeIs(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	session, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, worktreeCreateIn())
	if err != nil {
		t.Fatal(err)
	}
	path := "/home/u/.reet/worktrees/demo/site/" + storeRun
	for name, bad := range map[string][2]string{
		"relative path": {"site", "site"},
		"control":       {"/home/u/a\nb", "site"},
		"repo path":     {path, "../site"},
		"no repo":       {path, ""},
		"other run":     {"/home/u/.reet/worktrees/demo/site/fedcba9876543210fedcba98", "site"},
		"other project": {"/home/u/.reet/worktrees/other/site/" + storeRun, "site"},
		"other repo":    {"/home/u/.reet/worktrees/demo/other/" + storeRun, "site"},
		"outside reet":  {"/home/u/site", "site"},
	} {
		if err := s.RecordTerminalWorktreeFor(uid, a.ID, session.ID, bad[0], bad[1]); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(name, err)
		}
	}
	if err := s.RecordTerminalWorktreeFor(uid, a.ID, session.ID, path, "site"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordTerminalWorktreeFor(uid, a.ID, session.ID, path, "site"); err != nil {
		t.Fatal("same worktree again", err)
	}
	if err := s.RecordTerminalWorktreeFor(uid, a.ID, session.ID, "/home/v/.reet/worktrees/demo/site/"+storeRun, "site"); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("a different worktree replaced the recorded one", err)
	}
	got, err := s.TerminalSessionFor(uid, session.ID)
	if err != nil || got.WorktreePath != path || got.WorktreeRepo != "site" || got.WorktreeBranch != "reet/demo-"+storeRun || got.Dir != path {
		t.Fatal("recorded worktree", got, err)
	}
	data, _ := json.Marshal(got)
	for _, field := range []string{`"worktreePath":"` + path + `"`, `"worktreeRepo":"site"`, `"worktreeBranch":"reet/demo-` + storeRun + `"`} {
		if !strings.Contains(string(data), field) {
			t.Fatal(field, string(data))
		}
	}
	// A session made without a worktree cannot be given one, and shows none.
	plain := createTerminal(t, s, uid, a, c)
	if err := s.RecordTerminalWorktreeFor(uid, a.ID, plain.ID, path, "site"); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("worktree recorded on a plain session", err)
	}
	plainGot, _ := s.TerminalSessionFor(uid, plain.ID)
	if data, _ := json.Marshal(plainGot); strings.Contains(string(data), "worktree") {
		t.Fatal("plain session JSON", string(data))
	}
	// Another account cannot record on this session.
	other, err := s.CreateUser("other@example.com", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordTerminalWorktreeFor(other.ID, a.ID, session.ID, path, "site"); err == nil {
		t.Fatal("another account recorded a worktree")
	}
}

func TestRefusedWorktreeCreateKeepsItsReason(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	request := syncID()
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, worktreeCreateIn()); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]terminalproto.Result{
		"reason on another code": {Code: terminalproto.Capacity, Reason: terminalproto.WorktreeRepoBusy},
		"unknown reason":         {Code: terminalproto.WorktreeRefused, Reason: "nope"},
		"detail alone":           {Code: terminalproto.WorktreeRefused, Detail: "x"},
		"control in detail":      {Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeGitFailed, Detail: "a\nb"},
		"long detail":            {Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeGitFailed, Detail: strings.Repeat("x", 201)},
	} {
		if err := s.FailTerminalCreateWithResultFor(uid, a.ID, request, c.Generation, bad); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(name, err)
		}
	}
	refusal := terminalproto.Result{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeGitFailed, Detail: "fatal: invalid reference: HEAD"}
	if err := s.FailTerminalCreateWithResultFor(uid, a.ID, request, c.Generation, refusal); err != nil {
		t.Fatal(err)
	}
	if err := s.FailTerminalCreateWithResultFor(uid, a.ID, request, c.Generation, refusal); err != nil {
		t.Fatal("the same refusal again", err)
	}
	other := refusal
	other.Reason = terminalproto.WorktreeRepoBusy
	other.Detail = ""
	if err := s.FailTerminalCreateWithResultFor(uid, a.ID, request, c.Generation, other); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("a different reason replaced the stored one", err)
	}
	op, err := s.TerminalOperationFor(uid, a.ID, "create", request)
	if err != nil || op.Code != terminalproto.WorktreeRefused || op.Reason != terminalproto.WorktreeGitFailed || op.Detail != refusal.Detail {
		t.Fatal("stored refusal", op, err)
	}
}
