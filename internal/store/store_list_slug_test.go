package store

import (
	"strings"
	"testing"
)

func TestNewListSlugIsReadableAndUnique(t *testing.T) {
	s := newStore(t)
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateList(owner, "Design & UI picks", "", "private")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateList(owner, "Design & UI picks", "", "private")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.Slug, "design-ui-picks-") {
		t.Fatalf("slug = %q, want a readable prefix", first.Slug)
	}
	if first.Slug == second.Slug {
		t.Fatalf("two lists shared slug %q", first.Slug)
	}
	if strings.ContainsAny(first.Slug, " &") {
		t.Fatalf("slug = %q contains spaces or punctuation", first.Slug)
	}
	plain, err := s.CreateList(owner, "!!!", "", "private")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain.Slug, "list-") {
		t.Fatalf("empty name slug = %q, want list- prefix", plain.Slug)
	}
}
