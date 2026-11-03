package store

import (
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
)

func TestValidateHostedBundles(t *testing.T) {
	good := "---\nname: demo\ndescription: >-\n  A description\n  across lines.\n---\n\nInstructions.\n"
	valid := HostedSkill{Slug: "demo", Content: good}
	if err := ValidateHostedSkill(&valid); err != nil || valid.Description != "A description across lines." {
		t.Fatalf("valid frontmatter: %+v %v", valid, err)
	}
	cases := []HostedSkill{
		{Slug: "../bad", Content: good},
		{Slug: "other", Content: good},
		{Slug: "demo", Content: "# no frontmatter"},
		{Slug: "demo", Content: strings.Replace(good, "name: demo", "name: [demo]", 1)},
		{Slug: "demo", Content: strings.Replace(good, "name: demo", "name: demo\nname: demo", 1)},
		{Slug: "demo", Content: "---\nname: demo\ndescription: ok\n---\n"},
		{Slug: "demo", Content: good, Files: map[string][]byte{"SKILL.md": []byte("replace")}},
		{Slug: "demo", Content: good, Files: map[string][]byte{"../secret": nil}},
		{Slug: "demo", Content: good, Files: map[string][]byte{"/absolute": nil}},
		{Slug: "demo", Content: good, Files: map[string][]byte{"scripts\\bad.py": nil}},
		{Slug: "demo", Content: good, Files: map[string][]byte{"a/../bad": nil}},
		{Slug: "demo", Content: good, Files: map[string][]byte{".well-known/index.json": nil}},
		{Slug: "demo", Content: good, Files: map[string][]byte{"too-large": make([]byte, 2<<20)}},
	}
	for i, sk := range cases {
		if err := ValidateHostedSkill(&sk); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestHostedStorageSurvivesReopenAndShelfRestore(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("host@example.com", "Host", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ClaimPublishingUsername(u.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	sk := HostedSkill{Slug: "demo", Content: "---\nname: demo\ndescription: Description\n---\n\nInstructions\n", Published: true, Files: map[string][]byte{"asset.bin": {0, 255}}}
	if _, err := st.SaveHostedSkill(u.ID, sk); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.PublishedHostedSkill("alice", "demo")
	if err != nil || string(got.Files["asset.bin"]) != string([]byte{0, 255}) || got.CreatedAt == 0 {
		t.Fatalf("reopened: %+v %v", got, err)
	}
	restored := Skill{ID: "hosted/reet.sh/alice/demo", SourceType: "hosted", URL: "https://reet.sh/alice/demo"}
	if err := st.RestoreSnapshotFor(u.ID, Snapshot{Skills: []Skill{restored}}); err != nil {
		t.Fatal(err)
	}
	shelf, ok, err := st.GetFor(u.ID, restored.ID)
	if err != nil || !ok || shelf.SourceType != "hosted" || shelf.Source != "reet.sh/alice" {
		t.Fatalf("restore: %+v %v", shelf, err)
	}
	restored.URL = "https://other.example/alice/demo"
	if err := st.RestoreSnapshotFor(u.ID, Snapshot{Skills: []Skill{restored}}); err == nil {
		t.Fatal("accepted mismatched hosted URL")
	}
}

func TestHostedPrefixNormalization(t *testing.T) {
	good := map[string]string{"": "", "reet": "reet-", "Reet--": "reet-", " acme ": "acme-", "my-team-": "my-team-"}
	for in, want := range good {
		got, err := NormalizeHostedPrefix(in)
		if err != nil || got != want {
			t.Errorf("NormalizeHostedPrefix(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"bad name", "a--b", "-lead", "way-too-long-prefix-that-exceeds-the-limit"} {
		if _, err := NormalizeHostedPrefix(in); err == nil {
			t.Errorf("NormalizeHostedPrefix(%q) accepted", in)
		}
	}
}

func TestHostedDraftHidesItsShelfProjectionAndKeepsItsData(t *testing.T) {
	s := newStore(t)
	u, err := s.CreateUser("hide@example.com", "Hide", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimPublishingUsername(u.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	sk := HostedSkill{Slug: "demo", Content: "---\nname: demo\ndescription: Description\n---\n\nInstructions\n", Published: true}
	if _, err := s.SaveHostedSkill(u.ID, sk); err != nil {
		t.Fatal(err)
	}
	rf, err := ref.ParseAt("https://reet.sh/alice/demo", "reet.sh")
	if err != nil {
		t.Fatal(err)
	}
	test := true
	if _, err := s.UpsertFor(u.ID, rf, Patch{Bookmarked: &test}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHostedSkillVisible(rf.ID, true); err != nil {
		t.Fatal(err)
	}
	note := "Keep this note"
	if _, err := s.PatchFor(u.ID, rf.ID, Patch{Notes: &note}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSkillTagsFor(u.ID, rf.ID, []string{"keep"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.CreateList(u.ID, "Kit", "", "private")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddListSkill(list.ID, rf.ID, "", u.ID); err != nil {
		t.Fatal(err)
	}

	assertVisible := func(want int, when string) {
		t.Helper()
		shelf, counts, err := s.ListFor(u.ID, Filter{})
		if err != nil || len(shelf) != want || counts.All != want {
			t.Fatalf("%s: shelf=%+v counts=%+v err=%v", when, shelf, counts, err)
		}
		items, err := s.listSkills(list.ID)
		if err != nil || len(items) != want {
			t.Fatalf("%s: list=%+v err=%v", when, items, err)
		}
		shared, err := s.SharedListByID(list.ID)
		if err != nil || shared.SkillCount != want {
			t.Fatalf("%s: list count=%+v err=%v", when, shared, err)
		}
		open, openCounts, err := s.List(Filter{})
		if err != nil || len(open) != want || openCounts.All != want {
			t.Fatalf("%s: open shelf=%+v counts=%+v err=%v", when, open, openCounts, err)
		}
	}
	assertVisible(1, "published")
	if err := s.SetHostedSkillVisible(rf.ID, false); err != nil {
		t.Fatal(err)
	}
	assertVisible(0, "draft")
	renamedRef, err := ref.ParseAt("https://reet.sh/alice/demo-two", "reet.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameHostedSkill(u.ID, "demo", "demo-two", rf, renamedRef); err != nil {
		t.Fatal(err)
	}
	assertVisible(0, "renamed draft")
	if err := s.SetHostedSkillVisible(renamedRef.ID, true); err != nil {
		t.Fatal(err)
	}
	assertVisible(1, "renamed published")
	shelf, _, err := s.ListFor(u.ID, Filter{})
	if err != nil || shelf[0].Notes != note {
		t.Fatalf("notes lost across the draft and rename: %+v %v", shelf, err)
	}
	if err := s.SetHostedSkillVisible(renamedRef.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHostedSkill(u.ID, "demo-two", renamedRef.ID); err != nil {
		t.Fatal(err)
	}
	assertVisible(0, "deleted")
}
