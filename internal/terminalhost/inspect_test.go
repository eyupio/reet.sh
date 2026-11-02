package terminalhost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func inspectRepo(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	inspectGit(t, repo, "init", "-b", "main")
	inspectGit(t, repo, "config", "user.email", "fixture@example.test")
	inspectGit(t, repo, "config", "user.name", "Fixture")
	os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\ntwo\n"), 0600)
	inspectGit(t, repo, "add", ".")
	inspectGit(t, repo, "commit", "-m", "initial")
	return home, repo
}
func inspectGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}
func assertInspect(t *testing.T, r terminalproto.InspectResult) {
	t.Helper()
	_, err := terminalproto.Encode(terminalproto.Message{Version: 1, Generation: 1, RequestID: strings.Repeat("a", 16), Body: r})
	if err != nil {
		t.Fatalf("invalid result: %+v, %v", r, err)
	}
}
func TestInspectCheckoutAndWorktree(t *testing.T) {
	home, repo := inspectRepo(t)
	// External diff/fsmonitor helpers must never execute on a read.
	marker := filepath.Join(home, "executed")
	inspectGit(t, repo, "config", "core.fsmonitor", "touch "+marker)
	inspectGit(t, repo, "config", "diff.external", "touch "+marker)
	inspectGit(t, repo, "config", "filter.fixture.clean", "touch "+marker)
	inspectGit(t, repo, "config", "filter.fixture.required", "true")
	os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.txt filter=fixture\n"), 0600)
	// Ignore the attributes fixture itself in the changed-file count.
	os.WriteFile(filepath.Join(repo, ".git/info/exclude"), []byte(".gitattributes\n"), 0600)
	os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\nthree\nfour\n"), 0600)
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0600)
	r := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "status"})
	assertInspect(t, r)
	if r.Reason != "" || r.Branch != "main" || r.Changed != 2 || !r.CountsKnown {
		t.Fatalf("checkout %+v", r)
	}
	for _, f := range r.Files {
		if f.Path == "file.txt" && (f.Added != 2 || f.Removed != 1) {
			t.Fatalf("counts %+v", f)
		}
	}
	r = InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "diff", Path: "file.txt"})
	assertInspect(t, r)
	if !strings.Contains(r.Text, "+three") || !strings.Contains(r.Text, "-two") {
		t.Fatalf("diff %+v", r)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("configured helper executed")
	}
	inspectGit(t, repo, "config", "--unset", "core.fsmonitor")
	worktree := filepath.Join(home, "worktree")
	inspectGit(t, repo, "worktree", "add", "-b", "feature", worktree)
	os.WriteFile(filepath.Join(worktree, "file.txt"), []byte("edited\n"), 0600)
	r = InspectSession(t.Context(), home, terminalproto.Inspect{Dir: worktree, View: "status"})
	assertInspect(t, r)
	if r.Reason != "" || r.Branch != "feature" || r.Changed != 1 {
		t.Fatalf("worktree %+v", r)
	}
}
func TestInspectFileBoundsAndConfinement(t *testing.T) {
	home, repo := inspectRepo(t)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0600)
	os.Symlink(outside, filepath.Join(repo, "link"))
	os.WriteFile(filepath.Join(repo, "large.txt"), []byte(strings.Repeat("é", 10000)), 0600)
	os.WriteFile(filepath.Join(repo, "binary"), []byte{'a', 0, 'b'}, 0600)
	for _, p := range []string{"../secret", ".git/config", "link/secret"} {
		r := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "file", Path: p})
		if r.Reason == "" || r.Text != "" {
			t.Fatalf("path escaped %q %+v", p, r)
		}
		assertInspect(t, r)
	}
	r := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "file", Path: "large.txt"})
	assertInspect(t, r)
	if !r.More || len(r.Text) > 6500 || r.Reason != "" {
		t.Fatalf("large %+v", r)
	}
	r = InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "file", Path: "binary"})
	if !r.Binary || r.Text != "" {
		t.Fatalf("binary %+v", r)
	}
	r = InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "files"})
	assertInspect(t, r)
	for _, f := range r.Files {
		if f.Path == ".git" || f.Path == "link" {
			t.Fatalf("unsafe file listed %+v", f)
		}
	}
	r = InspectSession(t.Context(), home, terminalproto.Inspect{Dir: outside, View: "file", Path: "secret"})
	if r.Reason == "" {
		t.Fatal("outside home read")
	}
}
func TestInspectConflictsAndPartialClone(t *testing.T) {
	home, repo := inspectRepo(t)
	inspectGit(t, repo, "checkout", "-b", "other")
	os.WriteFile(filepath.Join(repo, "file.txt"), []byte("other\n"), 0600)
	inspectGit(t, repo, "commit", "-am", "other")
	inspectGit(t, repo, "checkout", "main")
	os.WriteFile(filepath.Join(repo, "file.txt"), []byte("main\n"), 0600)
	inspectGit(t, repo, "commit", "-am", "main")
	exec.Command("git", "-C", repo, "merge", "other").Run()
	r := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "status"})
	assertInspect(t, r)
	if r.Conflicts != 1 {
		t.Fatalf("conflicts %+v", r)
	}
	inspectGit(t, repo, "config", "remote.origin.promisor", "true")
	r = InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "status"})
	if r.Reason != "git_failed" {
		t.Fatalf("partial clone %+v", r)
	}
}
func TestInspectGitHubRemoteAndPriority(t *testing.T) {
	for _, remote := range []string{"git@github.com:owner/repo.git", "https://github.com/owner/repo.git"} {
		if got := inspectGitHubRepo(remote); got != "github.com/owner/repo" {
			t.Fatal(remote, got)
		}
	}
	for _, remote := range []string{"ext::touch /tmp/marker", "https://token@github.com/owner/repo", "https://evil.test/owner/repo", "https://github.com/owner/repo?token=x", "git@github.com:owner/../repo"} {
		if got := inspectGitHubRepo(remote); got != "" {
			t.Fatal(remote, got)
		}
	}
	if inspectCheckRank("FAILURE") >= inspectCheckRank("IN_PROGRESS") || inspectCheckRank("IN_PROGRESS") >= inspectCheckRank("SUCCESS") {
		t.Fatal("incorrect attention order")
	}
	// A deterministic worker answer exercises formatting without a network or credentials.
	home, repo := inspectRepo(t)
	bin := filepath.Join(home, "bin")
	os.Mkdir(bin, 0700)
	script := "#!/bin/sh\nprintf '%s' '{\"number\":7,\"title\":\"Build\",\"url\":\"https://github.com/o/r/pull/7\",\"state\":\"OPEN\",\"mergeable\":\"CONFLICTING\",\"statusCheckRollup\":[{\"name\":\"pass\",\"conclusion\":\"SUCCESS\"},{\"name\":\"fail\",\"conclusion\":\"FAILURE\"}]}'\n"
	os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	git := func(...string) ([]byte, error) { return []byte("https://github.com/o/r.git"), nil }
	r := inspectGitHub(context.Background(), repo, git, terminalproto.InspectResult{Branch: "main"})
	assertInspect(t, r)
	if r.Pull == nil || r.Pull.Checks[0].Name != "fail" || r.Pull.Mergeable != "CONFLICTING" {
		t.Fatalf("github %+v", r)
	}
}

func TestInspectListsFilesInsideUntrackedDirectories(t *testing.T) {
	home, repo := inspectRepo(t)
	path := filepath.Join(repo, "newdir", "nested")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "new.txt"), []byte("new content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "status"})
	assertInspect(t, r)
	if r.Reason != "" || r.Changed != 1 || len(r.Files) != 1 || r.Files[0].Path != "newdir/nested/new.txt" || r.Files[0].Status != "??" {
		t.Fatalf("untracked files missing: %+v", r)
	}
	preview := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "file", Path: r.Files[0].Path})
	if preview.Reason != "" || preview.Text != "new content\n" {
		t.Fatalf("untracked preview: %+v", preview)
	}
}

func TestInspectRenameCountsAndPreviewUseTheSameBaseline(t *testing.T) {
	for _, modified := range []bool{false, true} {
		name := "pure"
		if modified {
			name = "edited"
		}
		t.Run(name, func(t *testing.T) {
			home, repo := inspectRepo(t)
			original := strings.Repeat("unchanged line\n", 20) + "old tail\n"
			if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			inspectGit(t, repo, "commit", "-am", "long fixture")
			// Inspection must remain consistent even when the checkout disables rename detection.
			inspectGit(t, repo, "config", "status.renames", "false")
			inspectGit(t, repo, "config", "diff.renames", "false")
			inspectGit(t, repo, "mv", "file.txt", "renamed.txt")
			if modified {
				if err := os.WriteFile(filepath.Join(repo, "renamed.txt"), []byte(strings.Replace(original, "old tail", "new tail", 1)), 0600); err != nil {
					t.Fatal(err)
				}
				inspectGit(t, repo, "add", "renamed.txt")
			}
			r := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "status"})
			assertInspect(t, r)
			if r.Reason != "" || r.Changed != 1 || !r.CountsKnown || len(r.Files) != 1 || r.Files[0].Path != "renamed.txt" || !strings.Contains(r.Files[0].Status, "R") {
				t.Fatalf("rename status: %+v", r)
			}
			want := 0
			if modified {
				want = 1
			}
			if r.Files[0].Added != want || r.Files[0].Removed != want {
				t.Fatalf("rename counts: %+v", r.Files[0])
			}
			preview := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "diff", Path: "renamed.txt"})
			assertInspect(t, preview)
			if preview.Reason != "" || !strings.Contains(preview.Text, "rename from file.txt") || !strings.Contains(preview.Text, "rename to renamed.txt") || strings.Contains(preview.Text, "new file mode") {
				t.Fatalf("rename preview: %+v", preview)
			}
			if modified && (!strings.Contains(preview.Text, "-old tail") || !strings.Contains(preview.Text, "+new tail")) {
				t.Fatalf("edited rename preview: %+v", preview)
			}
		})
	}
}
