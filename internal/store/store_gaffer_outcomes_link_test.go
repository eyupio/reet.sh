package store

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A later pipeline stage starts in the worktree its first stage was given,
// which a machine makes under the home folder, not under the project's.
func TestLinkTerminalSessionProjectForFollowsTheFirstStagesWorktree(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	project, err := s.CreateFolderProjectFor(uid, FolderProject{Host: a.ID, Dir: "/home/u/site", Name: "demo", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	first := worktreeCreateIn()
	first.ProjectID = project.ID
	code, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, first)
	if err != nil {
		t.Fatal(err)
	}
	work := "/home/u/.reet/worktrees/demo/site/" + storeRun
	if err = s.RecordTerminalWorktreeFor(uid, a.ID, code.ID, work, "site"); err != nil {
		t.Fatal(err)
	}
	start := func(dir string) string {
		t.Helper()
		row, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, terminalproto.Create{Title: "Stage", Cols: 80, Rows: 24, Tool: "claude", Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	review := start(work)
	if err = s.LinkTerminalSessionProjectFor(uid, review, project.ID); err != nil {
		t.Fatal(err)
	}
	// Asked again, as a replayed launch does, it is left as it is.
	if err = s.LinkTerminalSessionProjectFor(uid, review, project.ID); err != nil {
		t.Fatal(err)
	}
	if row, err := s.TerminalSessionFor(uid, review); err != nil || row.ProjectID == nil || *row.ProjectID != project.ID {
		t.Fatalf("linked session: %+v %v", row.ProjectID, err)
	}
	for name, dir := range map[string]string{
		"another run's worktree": "/home/u/.reet/worktrees/demo/site/fedcba9876543210fedcba98",
		"another folder":         "/home/u/other",
		"the old layout":         "/home/u/site/.reet/worktrees/" + storeRun,
	} {
		if err := s.LinkTerminalSessionProjectFor(uid, start(dir), project.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal(name, err)
		}
	}
	other, err := s.CreateUser("link-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LinkTerminalSessionProjectFor(other.ID, review, project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("another account", err)
	}
}
