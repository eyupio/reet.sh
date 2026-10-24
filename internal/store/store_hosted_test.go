package store

import (
	"strings"
	"testing"
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
