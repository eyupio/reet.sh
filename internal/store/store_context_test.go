package store

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestContextGrantOwnershipRevisionAndConversationRemoval(t *testing.T) {
	s, uid, c := chatStoreFixture(t)
	other, err := s.CreateUser("other-context@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	g := ContextGrant{ContextFlags: ContextFlags{Status: true}}
	if _, err = s.SetContextGrantFor(other.ID, "gaffer", c.ID, 0, g); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign grant", err)
	}
	g, err = s.SetContextGrantFor(uid, "gaffer", c.ID, 0, g)
	if err != nil || g.Revision != 1 {
		t.Fatal(g, err)
	}
	if _, err = s.SetContextGrantFor(uid, "gaffer", c.ID, 0, g); !errors.Is(err, ErrGafferConflict) {
		t.Fatal("stale grant", err)
	}
	current, err := s.GafferConversationFor(uid, c.ID)
	if err != nil || current.Revision != c.Revision+1 {
		t.Fatal("sharing did not invalidate stale sends", current, err)
	}
	if err = s.DeleteGafferConversationFor(uid, c.ID, current.Revision); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM context_grants WHERE subject=?`, c.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("orphan grant", n, err)
	}
}

func TestContextMCPGrantsDoNotInheritShelfScope(t *testing.T) {
	s, uid, _ := chatStoreFixture(t)
	c, err := s.CreateMCPConnection(uid, "Read client", "write", "context-test-hash", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(c.ID, 10)
	g, err := s.ContextGrantFor(uid, "mcp", id, "")
	if err != nil || g.Any() || g.Revision != 0 {
		t.Fatal("implicit context access", g, err)
	}
	if err = s.CheckContextSubject(uid, "mcp", id, "", true); !errors.Is(err, ErrNotFound) {
		t.Fatal("paused master allowed context", err)
	}
	if err = s.SetMCPEnabled(uid, true); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckContextSubject(uid, "mcp", id, "", true); err != nil {
		t.Fatal(err)
	}
	g, err = s.SetContextGrantFor(uid, "mcp", id, 0, ContextGrant{ContextFlags: ContextFlags{Status: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RotateMCPConnection(uid, c.ID, "context-rotated-hash"); err != nil {
		t.Fatal(err)
	}
	again, err := s.ContextGrantFor(uid, "mcp", id, "")
	if err != nil || again.Revision != g.Revision {
		t.Fatal("rotation changed client-bound grant", again, err)
	}
	if err = s.RevokeMCPConnection(uid, c.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckContextSubject(uid, "mcp", id, "", true); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked client allowed context", err)
	}
	if err = s.CheckContextSubject(uid, "browser", "browser", "", true); !errors.Is(err, ErrNotFound) {
		t.Fatal("implicit browser access", err)
	}
}

func TestContextGrantCannotEnableGlobalHistoryOrPlainTextWithoutHistory(t *testing.T) {
	s, uid, c := chatStoreFixture(t)
	for _, flags := range []ContextFlags{{History: true}, {Text: true}, {History: true, Text: true}} {
		if _, err := s.SetContextGrantFor(uid, "gaffer", c.ID, 0, ContextGrant{ContextFlags: flags}); !errors.Is(err, ErrGafferConflict) {
			t.Fatal(flags, err)
		}
	}
	if err := s.CheckContextSubject(uid, "gaffer", strings.Repeat("e", 32), "", false); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckContextSubject(uid, "gaffer", c.ID, "", false); !errors.Is(err, ErrNotFound) {
		t.Fatal("demoted account retained access", err)
	}
}
