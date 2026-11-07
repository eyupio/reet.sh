package api

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestWorktreeRunTokenIsValidAndStable(t *testing.T) {
	a := worktreeRunToken("request-1")
	if !terminalproto.ValidRunToken(a) {
		t.Fatalf("invalid token %q", a)
	}
	if a != worktreeRunToken("request-1") || a == worktreeRunToken("request-2") {
		t.Fatal("token must follow the request id")
	}
}

func TestIsolateWorktreeOnlyTouchesRepoLaunches(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Skip("cannot open store:", err)
	}
	defer st.Close()
	s := &server{st: st}
	u, err := st.CreateUser("a@example.com", "a", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	// No saved repository: nothing to isolate, and nothing is invented.
	c := terminalproto.Create{Tool: "claude", Dir: "/home/a/reet"}
	if s.isolateWorktree(u.ID, "laptop", "req", &c) || c.Worktree != "" {
		t.Fatalf("unsaved folder was changed: %+v", c)
	}
	if _, err := st.CreateFolderProjectFor(u.ID, store.FolderProject{Host: "laptop", Dir: "/home/a/reet", Name: "Reet", IsRepo: true}); err != nil {
		t.Fatal(err)
	}
	// A saved repository gets a worktree, the same one on a retry.
	c = terminalproto.Create{Tool: "claude", Dir: "/home/a/reet", Project: ""}
	if !s.isolateWorktree(u.ID, "laptop", "req", &c) || c.Worktree != worktreeRunToken("req") || c.Project != "reet" || c.ProjectID == "" {
		t.Fatalf("repo launch not isolated: %+v", c)
	}
	for name, in := range map[string]terminalproto.Create{
		"shell":    {Dir: "/home/a/reet"},
		"named":    {Tool: "claude", Dir: "/home/a/reet", Worktree: worktreeRunToken("x"), Project: "p"},
		"resume":   {Tool: "claude", Dir: "/home/a/reet", Resume: true},
		"inside":   {Tool: "claude", Dir: "/home/a/.reet/worktrees/p/reet/" + worktreeRunToken("y")},
		"no dir":   {Tool: "claude"},
		"reopened": {Tool: "claude", Dir: "/home/a/reet", ResumeACP: "abc"},
	} {
		in := in
		if s.isolateWorktree(u.ID, "laptop", "req", &in) {
			t.Errorf("%s: should be left alone", name)
		}
	}
}
