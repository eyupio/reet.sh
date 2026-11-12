package store

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/ref"
)

// A private draft installed on a machine is hidden from the library but the
// machine inventory still says it is there. The session catalogue reads that
// inventory, so it must see the installed copy; ListFor keeps hiding it.
func TestInstalledSkillsForIncludesAHiddenInstalledSkill(t *testing.T) {
	e := installFixture(t)
	if _, err := e.s.InstallHostedSkillFor(e.uid, e.req("draft")); err != nil {
		t.Fatal(err)
	}
	rf, err := ref.ParseAt(installOrigin+"/publisher/draft", "reet.example")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := e.s.UpsertFor(e.uid, rf, Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.s.SetHostedSkillVisible(installed.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := e.s.RecordEnvironmentFor(e.uid, installed.ID, "laptop", "claude", "", ".claude/skills", ""); err != nil {
		t.Fatal(err)
	}

	got, err := e.s.InstalledSkillsFor(e.uid, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != installed.ID || got[0].Slug != "draft" {
		t.Fatalf("installed skills = %+v", got)
	}
	if len(got[0].Environments) != 1 || got[0].Environments[0].Host != "laptop" || got[0].Environments[0].Agent != "claude" {
		t.Fatalf("environments = %+v", got[0].Environments)
	}

	// A project-scoped install on the same host is part of the machine
	// inventory too; the session catalogue applies the folder rule afterwards.
	project, err := e.s.UpsertLocalFor(e.uid, "proj", Patch{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.s.RecordEnvironmentFor(e.uid, project.ID, "laptop", "claude", "/work/app", "/work/app/.claude/skills", ""); err != nil {
		t.Fatal(err)
	}
	got, err = e.s.InstalledSkillsFor(e.uid, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	foundProject := false
	for _, skill := range got {
		for _, env := range skill.Environments {
			if skill.ID == project.ID && env.Project == "/work/app" {
				foundProject = true
			}
		}
	}
	if !foundProject {
		t.Fatalf("project-scoped install missing from %+v", got)
	}

	// The library view is unchanged: a private draft stays hidden there.
	visible, _, err := e.s.ListFor(e.uid, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range visible {
		if skill.ID == installed.ID {
			t.Fatalf("ListFor now shows the hidden draft: %+v", visible)
		}
	}

	// Nothing is installed on another host or for another account.
	if other, err := e.s.InstalledSkillsFor(e.uid, "elsewhere"); err != nil || len(other) != 0 {
		t.Fatalf("other host = %+v, %v", other, err)
	}
	otherUser, err := e.s.CreateUser("installed-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if other, err := e.s.InstalledSkillsFor(otherUser.ID, "laptop"); err != nil || len(other) != 0 {
		t.Fatalf("other account = %+v, %v", other, err)
	}
}
