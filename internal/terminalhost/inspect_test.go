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

// fakeGH installs a gh that logs every invocation and answers `pr view` and
// `api graphql` from fixed files, so no test depends on a sign-in or the network.
func fakeGH(t *testing.T, view, graphql string, graphqlFails bool) (repo, log string) {
	t.Helper()
	home, repo := inspectRepo(t)
	bin := filepath.Join(home, "bin")
	os.Mkdir(bin, 0700)
	log = filepath.Join(home, "gh.log")
	os.WriteFile(filepath.Join(bin, "view.json"), []byte(view), 0600)
	os.WriteFile(filepath.Join(bin, "graphql.json"), []byte(graphql), 0600)
	fail := "cat " + filepath.Join(bin, "graphql.json")
	if graphqlFails {
		fail = "echo boom >&2; exit 1"
	}
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\ncase \"$1\" in pr) cat " + filepath.Join(bin, "view.json") + " ;; api) " + fail + " ;; esac\n"
	os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return repo, log
}

func ghCalls(t *testing.T, log string) []string {
	t.Helper()
	data, _ := os.ReadFile(log)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func inspectOriginRepo(t *testing.T, repo string) func(...string) ([]byte, error) {
	t.Helper()
	return func(...string) ([]byte, error) { return []byte("https://github.com/o/r.git"), nil }
}

const openView = `{"number":7,"title":"Build","url":"https://github.com/o/r/pull/7","state":"OPEN","mergeable":"MERGEABLE",
"createdAt":"2026-11-01T10:00:00Z","mergedAt":null,"closedAt":"0001-01-01T00:00:00Z","baseRefName":"main","headRefName":"feature","statusCheckRollup":[]}`

const openGraphQL = `{"data":{"repository":{"pullRequest":{
"latestReviews":{"nodes":[{"state":"APPROVED"},{"state":"CHANGES_REQUESTED"},{"state":"DISMISSED"}]},
"reviewRequests":{"totalCount":1},"autoMergeRequest":{"enabledAt":"2026-11-02T00:00:00Z"},
"reviewThreads":{"totalCount":5,"nodes":[{"isResolved":false},{"isResolved":false},{"isResolved":false},{"isResolved":true},{"isResolved":true}]},
"commits":{"nodes":[{"commit":{"committedDate":"2026-11-03T12:00:00Z"}}]}}}}}`

func TestAnOpenPullReadsReviewsThreadsAndLastPush(t *testing.T) {
	repo, log := fakeGH(t, openView, openGraphQL, false)
	r := inspectGitHub(context.Background(), repo, inspectOriginRepo(t, repo), terminalproto.InspectResult{Branch: "feature"})
	assertInspect(t, r)
	p := r.Pull
	if p == nil || !p.DetailKnown || p.Base != "main" || p.Head != "feature" || !p.AutoMerge {
		t.Fatalf("pull %+v", p)
	}
	if p.Reviews == nil || *p.Reviews != (terminalproto.InspectReviews{Approved: 1, ChangesRequested: 1, Requested: 1}) {
		t.Fatalf("reviews %+v", p.Reviews)
	}
	if p.Threads == nil || *p.Threads != (terminalproto.InspectThreads{Open: 3, Resolved: 2}) {
		t.Fatalf("threads %+v", p.Threads)
	}
	if p.CreatedAt != 1793527200000 || p.LastPushAt != 1793707200000 || p.MergedAt != 0 || p.ClosedAt != 0 {
		t.Fatalf("times %+v", p)
	}
	if r.Remote != "o/r" {
		t.Fatalf("remote %q", r.Remote)
	}
	if got := ghCalls(t, log); len(got) != 2 {
		t.Fatalf("calls %v", got)
	}
}

func TestThePullViewAsksOnlyForSmallFields(t *testing.T) {
	repo, log := fakeGH(t, openView, openGraphQL, false)
	inspectGitHub(context.Background(), repo, inspectOriginRepo(t, repo), terminalproto.InspectResult{Branch: "feature"})
	view := ghCalls(t, log)[0]
	for _, big := range []string{"latestReviews", "reviewRequests", "autoMergeRequest", "commits"} {
		if strings.Contains(view, big) {
			t.Fatalf("the pull view asks for %s, which can overrun the output cap: %s", big, view)
		}
	}
}

func TestTheGraphQLReadPassesTheRepositoryAsVariables(t *testing.T) {
	repo, log := fakeGH(t, openView, openGraphQL, false)
	inspectGitHub(context.Background(), repo, inspectOriginRepo(t, repo), terminalproto.InspectResult{Branch: "feature"})
	call := ghCalls(t, log)[1]
	for _, want := range []string{"api graphql", "-f owner=o", "-f name=r", "-F number=7"} {
		if !strings.Contains(call, want) {
			t.Fatalf("missing %q in %s", want, call)
		}
	}
	if strings.Contains(inspectPullQuery, `"o"`) || strings.Contains(inspectPullQuery, "number: 7") {
		t.Fatal("the query must be a constant")
	}
}

func TestAFailedThreadReadKeepsTheRest(t *testing.T) {
	repo, _ := fakeGH(t, openView, "", true)
	r := inspectGitHub(context.Background(), repo, inspectOriginRepo(t, repo), terminalproto.InspectResult{Branch: "feature"})
	assertInspect(t, r)
	p := r.Pull
	if p == nil || !p.DetailKnown || p.Threads != nil || p.Reviews != nil || p.AutoMerge || p.LastPushAt != 0 || p.Base != "main" {
		t.Fatalf("pull %+v", p)
	}
}

func TestANullPullRequestFromGraphQLLeavesDetailUnread(t *testing.T) {
	repo, _ := fakeGH(t, openView, `{"data":{"repository":{"pullRequest":null}}}`, false)
	r := inspectGitHub(context.Background(), repo, inspectOriginRepo(t, repo), terminalproto.InspectResult{Branch: "feature"})
	if r.Pull == nil || r.Pull.Threads != nil || r.Pull.Reviews != nil || !r.Pull.DetailKnown {
		t.Fatalf("pull %+v", r.Pull)
	}
}

func TestAMergedPullRequestMakesNoDetailRead(t *testing.T) {
	merged := strings.Replace(strings.Replace(openView, `"OPEN"`, `"MERGED"`, 1), `"mergedAt":null`, `"mergedAt":"2026-11-04T09:00:00Z"`, 1)
	repo, log := fakeGH(t, merged, openGraphQL, false)
	r := inspectGitHub(context.Background(), repo, inspectOriginRepo(t, repo), terminalproto.InspectResult{Branch: "feature"})
	if got := ghCalls(t, log); len(got) != 1 {
		t.Fatalf("calls %v", got)
	}
	if r.Pull.MergedAt != 1793782800000 || r.Pull.Threads != nil || r.Pull.Reviews != nil {
		t.Fatalf("pull %+v", r.Pull)
	}
}

func TestAReplyWithoutTheNewFieldsIsNotDetailKnown(t *testing.T) {
	old := `{"number":7,"title":"Build","url":"https://github.com/o/r/pull/7","state":"OPEN","statusCheckRollup":[]}`
	repo, log := fakeGH(t, old, openGraphQL, false)
	r := inspectGitHub(context.Background(), repo, inspectOriginRepo(t, repo), terminalproto.InspectResult{Branch: "feature"})
	if r.Pull.DetailKnown || len(ghCalls(t, log)) != 1 {
		t.Fatalf("pull %+v", r.Pull)
	}
}

func TestMoreThanAHundredThreadsAreMarked(t *testing.T) {
	nodes := strings.TrimSuffix(strings.Repeat(`{"isResolved":false},`, 100), ",")
	gql := `{"data":{"repository":{"pullRequest":{"latestReviews":{"nodes":[]},"reviewRequests":{"totalCount":0},"autoMergeRequest":null,"reviewThreads":{"totalCount":130,"nodes":[` + nodes + `]},"commits":{"nodes":[]}}}}}`
	repo, _ := fakeGH(t, openView, gql, false)
	r := inspectGitHub(context.Background(), repo, inspectOriginRepo(t, repo), terminalproto.InspectResult{Branch: "feature"})
	if r.Pull.Threads == nil || r.Pull.Threads.Open != 100 || !r.Pull.Threads.More || r.Pull.LastPushAt != 0 || r.Pull.AutoMerge {
		t.Fatalf("threads %+v", r.Pull)
	}
}

func TestRemoteIsReportedBeforeAnyPullRequestExists(t *testing.T) {
	home, repo := inspectRepo(t)
	inspectGit(t, repo, "remote", "add", "origin", "https://github.com/o/r.git")
	r := InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "status"})
	assertInspect(t, r)
	if r.Remote != "o/r" || r.Pull != nil {
		t.Fatalf("status %+v", r)
	}
	inspectGit(t, repo, "remote", "set-url", "origin", "https://evil.test/o/r")
	r = InspectSession(t.Context(), home, terminalproto.Inspect{Dir: repo, View: "status"})
	if r.Remote != "" {
		t.Fatalf("an ineligible origin was reported: %q", r.Remote)
	}
}

func TestTheInspectReplyStillFitsWithTheExtraFieldsAndManyChecks(t *testing.T) {
	r := terminalproto.InspectResult{Repo: true, Branch: "feature", Remote: "o/r", Pull: &terminalproto.InspectPull{
		Number: 7, Title: strings.Repeat("t", 300), URL: "https://github.com/o/r/pull/7", State: "OPEN",
		Base: strings.Repeat("b", 255), Head: strings.Repeat("h", 255), CreatedAt: 1, LastPushAt: 2, DetailKnown: true,
		Reviews: &terminalproto.InspectReviews{Approved: 1}, Threads: &terminalproto.InspectThreads{Open: 3},
	}}
	for i := 0; i < 40; i++ {
		r.Pull.Checks = append(r.Pull.Checks, terminalproto.InspectCheck{Name: strings.Repeat("c", 180), State: "SUCCESS", URL: "https://github.com/" + strings.Repeat("u", 400)})
	}
	fitInspect(&r)
	assertInspect(t, r)
	if r.Pull == nil || r.Pull.Reviews == nil || !r.Pull.More || len(r.Pull.Checks) == 0 {
		t.Fatalf("truncation lost the pull request: %+v", r.Pull)
	}
}
