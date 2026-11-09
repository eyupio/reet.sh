package store

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalPromptsOwnershipPersistenceAndConcurrentEditors(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	exact := "Fix the login redirect.\n\n{{project}} on {{branch}}.\n{{skill:review}}"
	in := TerminalPromptWrite{
		Title: " Fix the login redirect ", Description: "A careful bug fix", Body: exact,
		Category: "Fixes", Tool: "claude", Model: "opus", Effort: "high", Revision: 0,
	}
	item, err := s.SaveTerminalPromptFor(uid, "", in)
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "Fix the login redirect" || item.Body != exact || item.Revision != 1 || item.Category != "Fixes" {
		t.Fatalf("save %+v", item)
	}
	other, err := s.CreateUser("prompts-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := s.TerminalPromptsFor(other.ID); err != nil || len(rows) != 0 {
		t.Fatal("foreign list", rows, err)
	}
	foreign := in
	foreign.Revision = 1
	if _, err = s.SaveTerminalPromptFor(other.ID, item.ID, foreign); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("foreign update", err)
	}
	if err = s.DeleteTerminalPromptFor(other.ID, item.ID, 1); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("foreign delete", err)
	}
	reopened, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rows, err := reopened.TerminalPromptsFor(uid)
	if err != nil || len(rows) != 1 || rows[0].Body != exact {
		t.Fatal("persistence", rows, err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, handle := range []*Store{s, reopened} {
		wg.Go(func() {
			next := in
			next.Revision = 1
			next.Title = "Updated"
			_, err := handle.SaveTerminalPromptFor(uid, item.ID, next)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrTerminalConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent editors", success, conflict)
	}
	if err = s.DeleteTerminalPromptFor(uid, item.ID, 1); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("stale delete", err)
	}
	if err = s.DeleteTerminalPromptFor(uid, item.ID, 2); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalPromptValidationRolesAndLimit(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	base := TerminalPromptWrite{Title: "Prompt", Body: "Do the work"}
	for _, change := range []func(*TerminalPromptWrite){
		func(in *TerminalPromptWrite) { in.Title = "" },
		func(in *TerminalPromptWrite) { in.Title = strings.Repeat("x", 81) },
		func(in *TerminalPromptWrite) { in.Body = "" },
		func(in *TerminalPromptWrite) { in.Body = " \n " },
		func(in *TerminalPromptWrite) { in.Body = "bad\x1b[2J" },
		func(in *TerminalPromptWrite) { in.Body = "bad\x00" },
		func(in *TerminalPromptWrite) { in.Body = strings.Repeat("x", MaxTerminalPromptBytes+1) },
		func(in *TerminalPromptWrite) { in.Body = string([]byte{0xff}) },
		func(in *TerminalPromptWrite) { in.Description = strings.Repeat("x", MaxTerminalPromptNotes+1) },
		func(in *TerminalPromptWrite) { in.Category = "bad\x1b" },
		func(in *TerminalPromptWrite) { in.Tool = strings.Repeat("x", MaxTerminalPromptAgent+1) },
		func(in *TerminalPromptWrite) { in.Model = strings.Repeat("x", MaxTerminalPromptModel+1) },
		func(in *TerminalPromptWrite) { in.Effort = "high\n" },
	} {
		invalid := base
		change(&invalid)
		if _, err := s.SaveTerminalPromptFor(uid, "", invalid); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("invalid prompt accepted: %+v", invalid)
		}
	}
	for _, change := range []string{"role='viewer'", "role='owner',disabled_at=1"} {
		if _, err := s.db.Exec(`UPDATE users SET `+change+` WHERE id=?`, uid); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveTerminalPromptFor(uid, "", base); !errors.Is(err, ErrRemoteNotFound) {
			t.Fatal("role boundary", err)
		}
	}
	if _, err := s.db.Exec(`UPDATE users SET role='owner',disabled_at=0 WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	for range MaxTerminalPrompts {
		if _, err := s.SaveTerminalPromptFor(uid, "", base); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SaveTerminalPromptFor(uid, "", base); !errors.Is(err, ErrTerminalPromptLimit) {
		t.Fatal("limit", err)
	}
}
