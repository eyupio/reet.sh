package store

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalCommandsOwnershipPersistenceAndConcurrentEditors(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	exact := "  printf '%s\\n' 'hello'\n\tdf -h\r\n"
	item, err := s.SaveTerminalCommandFor(uid, "", " Disk usage ", exact, "Notes", 0)
	if err != nil || item.Name != "Disk usage" || item.Command != exact || item.Revision != 1 {
		t.Fatalf("save %+v %v", item, err)
	}
	other, err := s.CreateUser("commands-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := s.TerminalCommandsFor(other.ID); err != nil || len(rows) != 0 {
		t.Fatal("foreign list", rows, err)
	}
	if _, err = s.SaveTerminalCommandFor(other.ID, item.ID, "Changed", "true", "", 1); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("foreign update", err)
	}
	if err = s.DeleteTerminalCommandFor(other.ID, item.ID, 1); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("foreign delete", err)
	}
	reopened, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rows, err := reopened.TerminalCommandsFor(uid)
	if err != nil || len(rows) != 1 || rows[0].Command != exact {
		t.Fatal("persistence", rows, err)
	}
	// Two browser drafts with the same revision must never silently overwrite.
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, handle := range []*Store{s, reopened} {
		wg.Go(func() { _, err := handle.SaveTerminalCommandFor(uid, item.ID, "Updated", exact, "", 1); results <- err })
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
	if err = s.DeleteTerminalCommandFor(uid, item.ID, 1); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("stale delete", err)
	}
	if err = s.DeleteTerminalCommandFor(uid, item.ID, 2); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalCommandValidationRolesAndLimit(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	for _, command := range []string{"", " \n ", "bad\x1b[2J", "bad\x00", strings.Repeat("x", 16385), string([]byte{0xff})} {
		if _, err := s.SaveTerminalCommandFor(uid, "", "Command", command, "", 0); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("invalid text accepted: %v", err)
		}
	}
	for _, change := range []string{"role='viewer'", "role='owner',disabled_at=1"} {
		if _, err := s.db.Exec(`UPDATE users SET `+change+` WHERE id=?`, uid); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveTerminalCommandFor(uid, "", "Command", "true", "", 0); !errors.Is(err, ErrRemoteNotFound) {
			t.Fatal("role boundary", err)
		}
	}
	if _, err := s.db.Exec(`UPDATE users SET role='owner',disabled_at=0 WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	for range MaxTerminalCommands {
		if _, err := s.SaveTerminalCommandFor(uid, "", "Command", "true", "", 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SaveTerminalCommandFor(uid, "", "Overflow", "true", "", 0); !errors.Is(err, ErrTerminalCommandLimit) {
		t.Fatal("limit", err)
	}
}
