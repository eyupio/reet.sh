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

func TestPublishedHostedSkillsListsOnlyPublicEnabledAuthors(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	alice, err := st.CreateUser("alice@example.com", "Alice", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ClaimPublishingUsername(alice.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveHostedSkill(alice.ID, HostedSkill{Slug: "demo", Content: "---\nname: demo\ndescription: Demo\n---\n\nInstructions\n", Published: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveHostedSkill(alice.ID, HostedSkill{Slug: "draft", Content: "---\nname: draft\ndescription: Draft\n---\n\nInstructions\n"}); err != nil {
		t.Fatal(err)
	}
	bob, err := st.CreateUser("bob@example.com", "Bob", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ClaimPublishingUsername(bob.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveHostedSkill(bob.ID, HostedSkill{Slug: "hidden", Content: "---\nname: hidden\ndescription: Hidden\n---\n\nInstructions\n", Published: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserDisabled(bob.ID, true); err != nil {
		t.Fatal(err)
	}
	refs, err := st.PublishedHostedSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Username != "alice" || refs[0].Slug != "demo" {
		t.Fatalf("published refs = %+v", refs)
	}
}

// Hiding and deleting a written skill reach its catalogue row under every
// address the server was reached by, and nobody else's row.
func TestHostedHideAndDeleteCoverEveryAddress(t *testing.T) {
	s := newStore(t)
	alice, _ := s.CreateUser("a@example.com", "A", "member", "hash")
	malice, _ := s.CreateUser("m@example.com", "M", "member", "hash")
	if err := s.ClaimPublishingUsername(alice.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimPublishingUsername(malice.ID, "malice"); err != nil {
		t.Fatal(err)
	}
	sk := HostedSkill{Slug: "demo", Content: "---\nname: demo\ndescription: Description\n---\n\nInstructions\n", Published: true}
	for _, uid := range []int64{alice.ID, malice.ID} {
		if _, err := s.SaveHostedSkill(uid, sk); err != nil {
			t.Fatal(err)
		}
	}
	test := true
	add := func(uid int64, url, host string) ref.Ref {
		t.Helper()
		rf, err := ref.ParseAt(url, host)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpsertFor(uid, rf, Patch{Bookmarked: &test}); err != nil {
			t.Fatal(err)
		}
		return rf
	}
	public := add(alice.ID, "https://reet.example.com/alice/demo", "reet.example.com")
	local := add(alice.ID, "http://localhost:8080/alice/demo", "localhost:8080")
	add(alice.ID, "https://reet.example.com/alice/demo-two", "reet.example.com")
	add(malice.ID, "https://reet.example.com/malice/demo", "reet.example.com")
	hidden := func() map[string]bool {
		t.Helper()
		rows, err := s.db.Query(`SELECT id FROM skills WHERE hidden=1`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var id string
			rows.Scan(&id)
			out[id] = true
		}
		return out
	}
	refs, err := s.HostedSkillRefs(alice.ID, "demo")
	if err != nil || len(refs) != 2 {
		t.Fatalf("refs: %+v %v", refs, err)
	}
	if err := s.HideHostedSkill(alice.ID, "demo"); err != nil {
		t.Fatal(err)
	}
	if got := hidden(); len(got) != 2 || !got[public.ID] || !got[local.ID] {
		t.Fatalf("hidden after hide: %v", got)
	}
	if err := s.SetHostedSkillVisible(public.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHostedSkillVisible(local.ID, true); err != nil {
		t.Fatal(err)
	}
	// The id is of an address the skill was never published from.
	if err := s.DeleteHostedSkill(alice.ID, "demo", "hosted/127.0.0.1/alice/demo"); err != nil {
		t.Fatal(err)
	}
	if got := hidden(); len(got) != 2 || !got[public.ID] || !got[local.ID] {
		t.Fatalf("hidden after delete: %v", got)
	}
}
