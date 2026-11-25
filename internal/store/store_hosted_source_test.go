package store

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
)

const demoHosted = "---\nname: reet-demo\ndescription: A hosted skill.\n---\n\nFollow these instructions.\n"

// A skill someone else published is a source reet copies from; the author's
// own written skill is not a source for itself.
func TestForeignHostedSkillIsACopySourceButAnOwnOneIsNot(t *testing.T) {
	s := newStore(t)
	reader, _ := s.DefaultOwner()
	author, err := s.CreateUser("author@example.com", "Author", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ClaimPublishingUsername(author.ID, "author"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveHostedSkill(author.ID, HostedSkill{Slug: "reet-demo", Content: demoHosted, Published: true}); err != nil {
		t.Fatal(err)
	}
	r, err := ref.ParseAt("https://reet.test/author/reet-demo", "reet.test")
	if err != nil || r.SourceType != "hosted" {
		t.Fatalf("ref %+v %v", r, err)
	}
	if _, err = s.AddFor(author.ID, r, Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddFor(reader, r, Patch{}); err != nil {
		t.Fatal(err)
	}
	mine := skillStateNamed(t, s, author.ID, "reet-demo")
	theirs := skillStateNamed(t, s, reader, "reet-demo")
	if got, ok, err := s.SkillSourceFor(author.ID, mine.id); err != nil || ok {
		t.Fatalf("the author's own skill has a source: %+v %v %v", got, ok, err)
	}
	got, ok, err := s.SkillSourceFor(reader, theirs.id)
	if err != nil || !ok || got.SourceType != "hosted" || got.Source != "reet.test/author" || got.Slug != "reet-demo" {
		t.Fatalf("source = %+v %v %v", got, ok, err)
	}
	if needs, err := s.SkillsNeedingCopyFor(reader); err != nil || !reflect.DeepEqual(needs, []SkillCopyNeed{{Item: theirs.id, Name: "reet-demo", Kind: "fetch"}}) {
		t.Fatalf("needs = %+v %v", needs, err)
	}
	if n, err := s.MarkUnsourcedSkillsFor(reader); err != nil || n != 0 {
		t.Fatalf("marked %d unsourced, %v", n, err)
	}
	files, err := s.PublishedHostedFiles("author", "reet-demo")
	if err != nil || len(files) != 1 || files[0].Path != "SKILL.md" || string(files[0].Data) != demoHosted {
		t.Fatalf("files = %+v %v", files, err)
	}
	if _, err = s.SaveHostedSkill(author.ID, HostedSkill{Slug: "reet-demo", Content: demoHosted}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PublishedHostedFiles("author", "reet-demo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unpublished: %v", err)
	}
}
