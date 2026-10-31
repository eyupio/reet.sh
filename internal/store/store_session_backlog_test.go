package store

import (
	"database/sql"
	"errors"
	"testing"
)

func TestSessionBacklogOwnershipConcurrencyAndFrozenRetries(t *testing.T) {
	s := newStore(t)
	uid, other := projectUser(t, s, "backlog"), projectUser(t, s, "other-backlog")
	in := SessionDraft{Title: "Polish the board", Prompt: "Make the filters clearer", AgentID: "studio", Tool: "claude", Model: "sonnet", Images: []DraftImage{{Name: "reference.png", Data: "private-image-sentinel"}}}
	d, err := s.SaveSessionDraftFor(uid, in, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID == "" || d.Version != 1 {
		t.Fatalf("%+v", d)
	}
	if _, err := s.SessionDraftFor(other, d.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("other account read", err)
	}
	list, err := s.SessionDraftsFor(uid)
	if err != nil || len(list) != 1 || len(list[0].Images) != 0 || list[0].ImageCount != 1 {
		t.Fatalf("list leaks image bodies or loses counts: %+v %v", list, err)
	}
	d.Title = "Revised title"
	updated, err := s.SaveSessionDraftFor(uid, d, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveSessionDraftFor(uid, d, false); !errors.Is(err, ErrDraftConflict) {
		t.Fatal("stale update", err)
	}
	if err := s.DeleteSessionDraftFor(uid, d.ID, d.Version); !errors.Is(err, ErrDraftConflict) {
		t.Fatal("stale deletion", err)
	}
	if _, err := s.PrepareSessionDraftFor(uid, d.ID, d.Version, d.Prompt); !errors.Is(err, ErrDraftConflict) {
		t.Fatal("stale start", err)
	}
	frozen, err := s.PrepareSessionDraftFor(uid, d.ID, updated.Version, "Exact prompt with uploaded references")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.PrepareSessionDraftFor(uid, d.ID, updated.Version, "A concurrent browser's different image paths")
	if err != nil {
		t.Fatal(err)
	}
	if again.PreparedPrompt != frozen.PreparedPrompt || again.Version != frozen.Version {
		t.Fatal("retry changed the frozen launch")
	}
	if _, err := s.SaveSessionDraftFor(uid, frozen, false); !errors.Is(err, ErrDraftConflict) {
		t.Fatal("edit after dispatch", err)
	}
	if err := s.DeleteSessionDraftFor(uid, d.ID, frozen.Version); !errors.Is(err, ErrDraftConflict) {
		t.Fatal("delete during dispatch", err)
	}
	list, err = s.SessionDraftsFor(uid)
	if err != nil || !list[0].Starting || list[0].PreparedPrompt != "" {
		t.Fatalf("starting summary: %+v %v", list, err)
	}
	plain, err := s.SaveSessionDraftFor(uid, in, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSessionDraftFor(other, plain.ID, plain.Version); !errors.Is(err, ErrDraftConflict) {
		t.Fatal("other account deletion", err)
	}
	if err := s.DeleteSessionDraftFor(uid, plain.ID, plain.Version); err != nil {
		t.Fatal(err)
	}
}
