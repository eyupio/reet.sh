package store

import (
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
)

// A list's Add all puts every skill it holds into the library with the status
// the person chose, Active or Archived.
func TestAddListToShelfUsesStatus(t *testing.T) {
	for _, status := range []string{SkillActive, SkillArchived} {
		t.Run(status, func(t *testing.T) {
			s := newStore(t)
			ownerUser, err := s.CreateUser("lists-owner@example.com", "owner", "owner", "hash")
			if err != nil {
				t.Fatal(err)
			}
			readerUser, err := s.CreateUser("lists-reader@example.com", "reader", "member", "hash")
			if err != nil {
				t.Fatal(err)
			}
			owner, reader := ownerUser.ID, readerUser.ID
			list, err := s.CreateList(owner, "Picks", "", "private")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"a/b/one", "a/b/two"} {
				r, err := ref.Parse(id)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.UpsertFor(owner, r, Patch{}); err != nil {
					t.Fatal(err)
				}
				if err = s.AddListSkill(list.ID, id, "", owner); err != nil {
					t.Fatal(err)
				}
			}
			n, err := s.AddListToShelf(reader, list.ID, status)
			if err != nil || n != 2 {
				t.Fatalf("added %d: %v", n, err)
			}
			items, err := s.LibraryFor(reader)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 2 {
				t.Fatalf("library: %+v", items)
			}
			for _, it := range items {
				if it.Status != status {
					t.Fatalf("%s landed %s, want %s", it.Name, it.Status, status)
				}
			}
		})
	}
	s := newStore(t)
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddListToShelf(owner, 1, "to_test"); err == nil {
		t.Fatal("an unknown status was accepted")
	}
}

// A skill added straight into Archived was never Active anywhere: it has no
// grace period in which machines would still be sent it as Active.
func TestArchiveNewSkillHasNoGrace(t *testing.T) {
	s := newStore(t)
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	r, err := ref.Parse("a/b/fresh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpsertFor(owner, r, Patch{}); err != nil {
		t.Fatal(err)
	}
	id, err := libraryAliasID(s.db, owner, "shelf", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ArchiveNewSkillFor(owner, id, time.Now()); err != nil {
		t.Fatal(err)
	}
	var status, prev string
	if err = s.db.QueryRow(`SELECT status,prev_status FROM library_items WHERE id=?`, id).Scan(&status, &prev); err != nil {
		t.Fatal(err)
	}
	if status != SkillArchived || prev != "" {
		t.Fatalf("status %q, previous %q", status, prev)
	}
}
