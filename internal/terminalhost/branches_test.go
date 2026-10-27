//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// branchesArgv are the only three git calls a listing makes, in order.
var branchesArgv = [][]string{
	{"for-each-ref", "--count=201", "--sort=-committerdate", "--format=%(refname)", "refs/heads", "refs/remotes"},
	{"symbolic-ref", "--quiet", "--short", "HEAD"},
	{"symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"},
}

// branchesByNameArgv is the listing of a repository that can fetch objects
// lazily (a partial clone): by name, which reads no object, instead of by
// commit date, which would make git fetch a missing commit.
var branchesByNameArgv = []string{"for-each-ref", "--count=201", "--sort=refname", "--format=%(refname)", "refs/heads", "refs/remotes"}

// cannedGit answers the three listing calls with fixed output; any other
// call fails.
func cannedGit(refs, head, originHead string) GitRunner {
	return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		switch {
		case reflect.DeepEqual(args, branchesArgv[0]), reflect.DeepEqual(args, branchesByNameArgv):
			return []byte(refs), nil
		case reflect.DeepEqual(args, branchesArgv[1]):
			if head == "" {
				return nil, errors.New("exit status 1")
			}
			return []byte(head + "\n"), nil
		case reflect.DeepEqual(args, branchesArgv[2]):
			if originHead == "" {
				return nil, errors.New("exit status 1")
			}
			return []byte(originHead + "\n"), nil
		}
		return nil, fmt.Errorf("unexpected git call %q", args)
	}
}

func requireBranchesEncodable(t *testing.T, r terminalproto.BranchesResult) {
	t.Helper()
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: "branches-test-request-01", Generation: 1, Body: r}); err != nil {
		t.Fatalf("branches result does not encode: %v (%+v)", err, r)
	}
}

func TestListBranchesArgv(t *testing.T) {
	t.Run("recording runner", func(t *testing.T) {
		home := browseHome(t)
		repo := filepath.Join(home, "code", "site")
		makeRepo(t, repo)
		calls, git := recordingGit(cannedGit("refs/heads/main\nrefs/remotes/origin/HEAD\nrefs/remotes/origin/main\n", "main", "origin/main"))
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, git)
		want := []gitCall{{repo, branchesArgv[0]}, {repo, branchesArgv[1]}, {repo, branchesArgv[2]}}
		if got := calls(); !reflect.DeepEqual(got, want) {
			t.Fatalf("git calls\n got %q\nwant %q", got, want)
		}
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Default: "main", Branches: []string{"main", "origin/main"}}) {
			t.Fatalf("%+v", r)
		}
		requireBranchesEncodable(t, r)
	})

	// A partial clone is listed by name: the same three calls, the first
	// sorted by refname.
	t.Run("recording runner, partial clone", func(t *testing.T) {
		home := browseHome(t)
		t.Setenv("HOME", home)
		repo := filepath.Join(home, "code", "site")
		makeRepo(t, repo)
		putFile(t, filepath.Join(repo, ".git", "config"), "[extensions]\n\tpartialClone = origin\n")
		calls, git := recordingGit(cannedGit("refs/heads/main\nrefs/remotes/origin/main\n", "main", ""))
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, git)
		want := []gitCall{{repo, branchesByNameArgv}, {repo, branchesArgv[1]}, {repo, branchesArgv[2]}}
		if got := calls(); !reflect.DeepEqual(got, want) {
			t.Fatalf("git calls\n got %q\nwant %q", got, want)
		}
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Branches: []string{"main", "origin/main"}}) {
			t.Fatalf("%+v", r)
		}
	})

	// Through ExecGit to a stand-in git on PATH: the argument vector is
	// exactly -C <dir> and the three calls, and the worktree environment
	// reaches it (no GIT_* variable, nothing else from this process).
	t.Run("worktree environment", func(t *testing.T) {
		home := browseHome(t)
		repo := filepath.Join(home, "site")
		makeRepo(t, repo)
		record := filepath.Join(shortTempDir(t), "record")
		script := `printf '%s\0' "$@" >> '` + record + `'
printf 'GIT_DIR=%s GIT_CEILING_DIRECTORIES=%s GIT_NO_LAZY_FETCH=%s GIT_CONFIG_PARAMETERS=%s REET_PROBE=%s HOME=%s\n' "${GIT_DIR-unset}" "${GIT_CEILING_DIRECTORIES-unset}" "${GIT_NO_LAZY_FETCH-unset}" "${GIT_CONFIG_PARAMETERS-unset}" "${REET_PROBE-unset}" "${HOME-unset}" >> '` + record + `'
case "$3" in
for-each-ref) printf 'refs/heads/main\nrefs/heads/feature/x\n' ;;
symbolic-ref) if [ "$6" = HEAD ]; then echo main; else exit 1; fi ;;
esac
`
		git := fakeGit(t, script)
		t.Setenv("PATH", filepath.Dir(git))
		t.Setenv("GIT_DIR", "/nowhere")
		t.Setenv("GIT_CEILING_DIRECTORIES", "/nowhere")
		t.Setenv("GIT_NO_LAZY_FETCH", "0")
		t.Setenv("GIT_CONFIG_PARAMETERS", "'core.pager=evil'")
		t.Setenv("REET_PROBE", "leak")
		t.Setenv("HOME", home)
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, ExecGit)
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Branches: []string{"main", "feature/x"}}) {
			t.Fatalf("%+v", r)
		}
		data, err := os.ReadFile(record)
		if err != nil {
			t.Fatal(err)
		}
		// git is pinned to this repository's own .git and may not look
		// above it, so it can never list another repository's branches.
		// Lazy fetching is off: git never fetches a missing object.
		env := "GIT_DIR=" + filepath.Join(repo, ".git") + " GIT_CEILING_DIRECTORIES=" + home + " GIT_NO_LAZY_FETCH=1 GIT_CONFIG_PARAMETERS=unset REET_PROBE=unset HOME=" + home + "\n"
		var want strings.Builder
		for _, args := range branchesArgv {
			want.WriteString(strings.Join(append([]string{"-C", repo}, args...), "\x00") + "\x00" + env)
		}
		if string(data) != want.String() {
			t.Fatalf("git saw\n%q\nwant\n%q", data, want.String())
		}
	})

	t.Run("filter is applied here, never passed to git", func(t *testing.T) {
		home := browseHome(t)
		repo := filepath.Join(home, "site")
		makeRepo(t, repo)
		filter := "$(touch-pwned);X"
		calls, git := recordingGit(cannedGit("refs/heads/main\nrefs/heads/Feature/$(Touch-PWNED);x-1\nrefs/heads/other\n", "main", ""))
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo, Filter: filter}, git)
		for _, c := range calls() {
			for _, a := range c.args {
				if strings.Contains(a, "pwned") {
					t.Fatal("the filter reached git", c.args)
				}
			}
		}
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Branches: []string{"Feature/$(Touch-PWNED);x-1"}}) {
			t.Fatalf("%+v", r)
		}
	})
}

// gitAt runs real git for setup; a commit made with when > 0 is dated that
// many seconds after now, so it is newer than anything newRepo made.
func gitAt(t *testing.T, dir string, when int, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0"}
	if when > 0 {
		at := time.Now().Unix() + int64(when)
		env = append(env, fmt.Sprintf("GIT_COMMITTER_DATE=%d +0000", at), fmt.Sprintf("GIT_AUTHOR_DATE=%d +0000", at))
	}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, out)
	}
}

func TestListBranchesRealGit(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "site")
	// main is the oldest; each later branch gets a newer commit.
	for i, name := range []string{"older", "newer", "newest"} {
		gitAt(t, repo, 0, "checkout", "-q", "-b", name, "main")
		gitAt(t, repo, 1000*(i+1), "commit", "-q", "--allow-empty", "-m", name)
	}
	gitAt(t, repo, 0, "update-ref", "refs/remotes/origin/trunk", "older")
	gitAt(t, repo, 0, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	gitAt(t, repo, 0, "checkout", "-q", "newer")

	r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, ExecGit)
	want := terminalproto.BranchesResult{Current: "newer", Default: "trunk", Branches: []string{"newest", "newer", "older", "origin/trunk", "main"}}
	// older and origin/trunk share a commit; git orders ties by refname.
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("newest first\n got %+v\nwant %+v", r, want)
	}
	requireBranchesEncodable(t, r)

	t.Run("filter", func(t *testing.T) {
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo, Filter: "NEW"}, ExecGit)
		if !reflect.DeepEqual(r.Branches, []string{"newest", "newer"}) || r.Current != "newer" || r.Default != "trunk" || r.More {
			t.Fatalf("%+v", r)
		}
	})

	t.Run("detached HEAD and no origin/HEAD", func(t *testing.T) {
		gitAt(t, repo, 0, "checkout", "-q", "--detach", "newest")
		gitAt(t, repo, 0, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, ExecGit)
		if r.Current != "" || r.Default != "" || r.Reason != "" || len(r.Branches) != 5 {
			t.Fatalf("%+v", r)
		}
		requireBranchesEncodable(t, r)
	})

	t.Run("201 refs list 200 and More", func(t *testing.T) {
		var stdin strings.Builder
		for i := range 196 {
			fmt.Fprintf(&stdin, "create refs/heads/bulk/%03d main\n", i)
		}
		cmd := exec.Command("git", "-C", repo, "update-ref", "--stdin")
		cmd.Stdin = strings.NewReader(stdin.String())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
		// 5 + 196 = 201 refs.
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, ExecGit)
		if len(r.Branches) != terminalproto.MaxBranchNames || !r.More || r.Reason != "" {
			t.Fatalf("got %d names, More %t, reason %q", len(r.Branches), r.More, r.Reason)
		}
		if r.Branches[0] != "newest" {
			t.Fatal("not newest first", r.Branches[:3])
		}
		requireBranchesEncodable(t, r)
		// With exactly 200 refs nothing is cut.
		gitAt(t, repo, 0, "update-ref", "-d", "refs/heads/bulk/000")
		r = ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, ExecGit)
		if len(r.Branches) != terminalproto.MaxBranchNames || r.More {
			t.Fatalf("got %d names, More %t", len(r.Branches), r.More)
		}
	})

	t.Run("git that fails leaks nothing", func(t *testing.T) {
		failing := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
			return []byte("fatal: secret detail /etc/shadow"), errors.New("exit status 128")
		}
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, failing)
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed}) {
			t.Fatalf("%+v", r)
		}
		t.Setenv("PATH", shortTempDir(t))
		r = ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, ExecGit)
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed}) {
			t.Fatalf("git missing: %+v", r)
		}
	})
}

// A listing is bounded by 5 seconds in all, whatever ctx allows.
func TestListBranchesTimeout(t *testing.T) {
	home := browseHome(t)
	repo := filepath.Join(home, "site")
	makeRepo(t, repo)
	var deadline time.Time
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		deadline, _ = ctx.Deadline()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	start := time.Now()
	ctx, stop := context.WithCancel(ctx)
	go func() { time.Sleep(50 * time.Millisecond); stop() }()
	r := ListBranches(ctx, home, terminalproto.Branches{Dir: repo}, git)
	if r.Reason != terminalproto.BranchesGitFailed {
		t.Fatalf("%+v", r)
	}
	if d := deadline.Sub(start); d > 5*time.Second+time.Second || d < 4*time.Second {
		t.Fatal("git was not bounded by 5 seconds", d)
	}
	if branchesTimeout != 5*time.Second {
		t.Fatal(branchesTimeout)
	}
}

// Names git accepts but the protocol cannot carry are skipped, never sent:
// one would make the whole answer invalid.
func TestListBranchesSkipsNamesThatCannotBeSent(t *testing.T) {
	home := browseHome(t)
	repo := filepath.Join(home, "site")
	makeRepo(t, repo)
	long := strings.Repeat("l", 255)
	refs := strings.Join([]string{
		"refs/heads/main",
		"refs/heads/@",
		"refs/heads/a\u202eb",
		"refs/heads/zero\u200bwidth",
		"refs/heads/line\u2028sep",
		"refs/heads/with space",
		"refs/heads/HEAD",
		"refs/remotes/origin/HEAD",
		"refs/remotes/origin/main",
		"refs/heads/origin/main", // the same short name as the remote branch
		"refs/heads/" + long + "x",
		"refs/heads/" + long,
		"refs/heads/-dash",
		"refs/tags/v1",
		"not-a-ref",
		"refs/heads/\xff",
		"refs/heads/ok",
		"",
	}, "\n") + "\n"
	r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, cannedGit(refs, "a\u202eb", "origin/with space"))
	want := terminalproto.BranchesResult{Branches: []string{"main", "origin/main", long, "ok"}}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("\n got %q\nwant %q", r.Branches, want.Branches)
	}
	requireBranchesEncodable(t, r)
	if r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, cannedGit("refs/heads/main\n", "HEAD", "origin/HEAD")); r.Current != "" || r.Default != "" {
		t.Fatalf("HEAD is never a branch: %+v", r)
	}
	if r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, cannedGit("refs/heads/main\n", "main\nextra", "upstream/main")); r.Current != "" || r.Default != "" {
		t.Fatalf("odd symbolic-ref output: %+v", r)
	}

	t.Run("the answer fits the frame", func(t *testing.T) {
		var lines []string
		for i := range 200 {
			lines = append(lines, fmt.Sprintf("refs/heads/%03d-%s", i, strings.Repeat("<", 250)))
		}
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, cannedGit(strings.Join(lines, "\n")+"\n", "main", "origin/main"))
		if r.Reason != "" || !r.More || len(r.Branches) == 0 || len(r.Branches) >= 200 {
			t.Fatalf("got %d names, More %t, reason %q", len(r.Branches), r.More, r.Reason)
		}
		requireBranchesEncodable(t, r)
		// Every name that fits is there, in order.
		for i, name := range r.Branches {
			if name != lines[i][len("refs/heads/"):] {
				t.Fatal("order", i, name)
			}
		}
	})
}

func TestListBranchesRefusesNonRoot(t *testing.T) {
	home := browseHome(t)
	outside := browseHome(t)
	repo := filepath.Join(home, "site")
	makeRepo(t, repo)
	mkdirs(t, repo, "sub")
	mkdirs(t, home, "plain", "linked-worktree", "git-link", "no-head/.git", "head-dir/.git/HEAD")
	putFile(t, filepath.Join(home, "linked-worktree", ".git"), "gitdir: "+filepath.Join(repo, ".git"))
	symlink(t, filepath.Join(repo, ".git"), filepath.Join(home, "git-link", ".git"))
	putFile(t, filepath.Join(home, "a-file"), "x")
	outRepo := filepath.Join(outside, "repo")
	makeRepo(t, outRepo)
	symlink(t, outRepo, filepath.Join(home, "to-outside-repo"))
	symlink(t, outside, filepath.Join(home, "to-outside"))
	symlink(t, repo, filepath.Join(home, "to-site"))

	for _, tc := range []struct {
		dir  string
		want terminalproto.BranchesReason
	}{
		{filepath.Join(repo, "sub"), terminalproto.BranchesNotRepoRoot},
		{home, terminalproto.BranchesNotRepoRoot},
		{filepath.Join(home, "plain"), terminalproto.BranchesNotRepoRoot},
		{filepath.Join(home, "linked-worktree"), terminalproto.BranchesNotRepoRoot},
		{filepath.Join(home, "git-link"), terminalproto.BranchesNotRepoRoot},
		{filepath.Join(home, "no-head"), terminalproto.BranchesNotRepoRoot},
		{filepath.Join(home, "head-dir"), terminalproto.BranchesNotRepoRoot},
		{filepath.Join(home, "a-file"), terminalproto.BranchesNotRepoRoot},
		{filepath.Join(home, "missing"), terminalproto.BranchesNotRepoRoot},
		{outRepo, terminalproto.BranchesOutsideHome},
		{outside, terminalproto.BranchesOutsideHome},
		{filepath.Join(outside, "missing"), terminalproto.BranchesOutsideHome},
		{filepath.Join(home, "to-outside-repo"), terminalproto.BranchesOutsideHome},
		{filepath.Join(home, "to-outside", "repo"), terminalproto.BranchesOutsideHome},
		{filepath.Join(home, "to-outside", "missing"), terminalproto.BranchesOutsideHome},
		{filepath.Join(home, "..", filepath.Base(outside), "repo"), terminalproto.BranchesOutsideHome},
		{"/", terminalproto.BranchesOutsideHome},
		{"relative/site", terminalproto.BranchesOutsideHome},
	} {
		calls, git := recordingGit(cannedGit("refs/heads/main\n", "main", ""))
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: tc.dir}, git)
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Reason: tc.want}) {
			t.Errorf("%s: got %+v want %s", tc.dir, r, tc.want)
		}
		if c := calls(); len(c) != 0 {
			t.Errorf("%s: git ran %q", tc.dir, c)
		}
	}

	// A repository root reached through a link inside home is listed, and
	// git runs in the resolved folder.
	calls, git := recordingGit(cannedGit("refs/heads/main\n", "main", ""))
	r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: filepath.Join(home, "to-site")}, git)
	if !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Branches: []string{"main"}}) {
		t.Fatalf("%+v", r)
	}
	for _, c := range calls() {
		if c.dir != repo {
			t.Fatal("git ran outside the resolved repository", c.dir)
		}
	}
	// A home reached through a link works the same.
	linkedHome := filepath.Join(outside, "home-link")
	symlink(t, home, linkedHome)
	r = ListBranches(context.Background(), linkedHome, terminalproto.Branches{Dir: filepath.Join(linkedHome, "site")}, cannedGit("refs/heads/main\n", "main", ""))
	if !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Branches: []string{"main"}}) {
		t.Fatalf("linked home: %+v", r)
	}

	// A repository swapped for a link out of home while git reads it is not
	// answered for.
	swapped := false
	swap := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if !swapped {
			swapped = true
			if err := os.Rename(repo, repo+"-moved"); err != nil {
				return nil, err
			}
			if err := os.Symlink(outRepo, repo); err != nil {
				return nil, err
			}
		}
		return cannedGit("refs/heads/main\n", "main", "")(ctx, dir, args...)
	}
	r = ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, swap)
	if !swapped || !reflect.DeepEqual(r, terminalproto.BranchesResult{Reason: terminalproto.BranchesOutsideHome}) {
		t.Fatalf("swapped mid-listing: %+v", r)
	}
}

// More is set whenever git gave 201 lines, even when skipped names or the
// filter leave fewer than 200 to list: refs past git's count may exist.
func TestListBranchesMoreWhenGitWasCut(t *testing.T) {
	home := browseHome(t)
	repo := filepath.Join(home, "site")
	makeRepo(t, repo)
	lines := []string{"refs/remotes/origin/HEAD"}
	for i := range 200 {
		lines = append(lines, fmt.Sprintf("refs/heads/b%03d", i))
	}
	refs := strings.Join(lines, "\n") + "\n"
	r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, cannedGit(refs, "b000", ""))
	if len(r.Branches) != 200 || !r.More {
		t.Fatalf("got %d names, More %t", len(r.Branches), r.More)
	}
	r = ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo, Filter: "b19"}, cannedGit(refs, "b000", ""))
	if len(r.Branches) != 10 || !r.More {
		t.Fatalf("filtered: got %d names, More %t", len(r.Branches), r.More)
	}
	// 200 lines: nothing was cut.
	r = ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo, Filter: "b19"}, cannedGit(strings.Join(lines[1:], "\n")+"\n", "b000", ""))
	if len(r.Branches) != 10 || r.More {
		t.Fatalf("not cut: got %d names, More %t", len(r.Branches), r.More)
	}
}

// A .git folder git does not accept (HEAD alone, or a garbage HEAD) must
// never make git look further up and list the repository around it, here
// one above home. git is pinned to <dir>/.git, so it fails instead.
func TestListBranchesNeverListsAnotherRepository(t *testing.T) {
	top := worktreeHome(t)
	outer := newRepo(t, top, "outer")
	testGit(t, outer, "branch", "secret-outside-home")
	home := filepath.Join(outer, "home")
	proj := filepath.Join(home, "proj")
	mkdirs(t, proj, ".git")
	for _, head := range []string{"ref: refs/heads/main\n", "garbage"} {
		putFile(t, filepath.Join(proj, ".git", "HEAD"), head)
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: proj}, ExecGit)
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed}) {
			t.Fatalf("HEAD %q: %+v", head, r)
		}
	}
	// The same inside a real repository nested in home.
	inner := newRepo(t, home, "inner")
	testGit(t, inner, "branch", "secret-inner")
	nested := filepath.Join(inner, "nested")
	mkdirs(t, nested, ".git")
	putFile(t, filepath.Join(nested, ".git", "HEAD"), "ref: refs/heads/main\n")
	if r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: nested}, ExecGit); r.Reason != terminalproto.BranchesGitFailed {
		t.Fatalf("nested: %+v", r)
	}
	// A real repository inside home still lists its own branches only.
	if r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: inner}, ExecGit); !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Branches: []string{"main", "secret-inner"}}) && !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Branches: []string{"secret-inner", "main"}}) {
		t.Fatalf("real repository: %+v", r)
	}
	// A linked worktree checkout (.git file) is still refused, git or not.
	linked := filepath.Join(home, "linked")
	testGit(t, inner, "worktree", "add", "-q", "-b", "wt", linked)
	if r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: linked}, ExecGit); !reflect.DeepEqual(r, terminalproto.BranchesResult{Reason: terminalproto.BranchesNotRepoRoot}) {
		t.Fatalf("worktree checkout: %+v", r)
	}
}

// A partial clone with a missing commit: sorting by commit date would make
// git fetch it, running the repository's configured ssh command. The listing
// must never run it (the marker stays absent) and still list the branches,
// by name.
func TestListBranchesNeverFetchesLazily(t *testing.T) {
	home := worktreeHome(t)
	repo := newRepo(t, home, "partial")
	marker := filepath.Join(shortTempDir(t), "marker")
	ssh := filepath.Join(shortTempDir(t), "ssh")
	if err := os.WriteFile(ssh, []byte("#!/bin/sh\ntouch '"+marker+"'\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "config", "core.repositoryformatversion", "1")
	testGit(t, repo, "config", "remote.origin.url", "ssh://example.invalid/repo")
	testGit(t, repo, "config", "core.sshCommand", ssh)
	missing := "1111111111111111111111111111111111111111"
	for _, tc := range []struct {
		name  string
		setup func()
	}{
		{"partialclone and promisor remote", func() {
			testGit(t, repo, "config", "extensions.partialclone", "origin")
			testGit(t, repo, "config", "remote.origin.promisor", "true")
		}},
		{"promisor remote only", func() {
			testGit(t, repo, "config", "--unset", "extensions.partialclone")
		}},
	} {
		name := tc.name
		tc.setup()
		putFile(t, filepath.Join(repo, ".git", "refs", "heads", "ghost"), missing+"\n")
		// The hazard is real on this git: sorting by date runs the command.
		if _, err := testGitErr(repo, "for-each-ref", "--sort=-committerdate", "refs/heads"); err == nil {
			t.Log(name, ": git sorted by date without failing")
		}
		if _, err := os.Stat(marker); err != nil {
			t.Logf("%s: this git did not fetch lazily for a date sort; checking the listing anyway", name)
		}
		os.Remove(marker)
		r := ListBranches(context.Background(), home, terminalproto.Branches{Dir: repo}, ExecGit)
		if _, err := os.Stat(marker); err == nil {
			t.Fatal(name, ": the listing ran the repository's ssh command")
		}
		if !reflect.DeepEqual(r, terminalproto.BranchesResult{Current: "main", Branches: []string{"ghost", "main"}}) {
			t.Fatalf("%s: %+v", name, r)
		}
		requireBranchesEncodable(t, r)
	}
}

// Whether a repository can fetch lazily is read from files, without git:
// its own config, its promisor packs, and the user's git config files.
func TestPromisorRepositoryIsSeenWithoutGit(t *testing.T) {
	for name, tc := range map[string]struct {
		config, global string
		files          map[string]string
		want           bool
	}{
		"plain":                    {config: "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = x\n", want: false},
		"no config":                {want: false},
		"partial clone":            {config: "[extensions]\n\tpartialClone = origin\n", want: true},
		"promisor remote":          {config: "[remote \"origin\"]\n\tpromisor = true\n", want: true},
		"upper case":               {config: "[Remote \"o\"]\n\tPROMISOR = TRUE\n", want: true},
		"include":                  {config: "[include]\n\tpath = other\n", want: true},
		"includeIf":                {config: "[includeIf \"gitdir:~/\"]\n\tpath = other\n", want: true},
		"promisor pack":            {files: map[string]string{"objects/pack/pack-1.promisor": ""}, want: true},
		"promisor in ~/.gitconfig": {global: "[remote \"o\"]\n\tpromisor = true\n", want: true},
		"plain ~/.gitconfig":       {global: "[user]\n\tname = x\n", want: false},
	} {
		home := browseHome(t)
		t.Setenv("HOME", home)
		repo := filepath.Join(home, "site")
		makeRepo(t, repo)
		if tc.config != "" {
			putFile(t, filepath.Join(repo, ".git", "config"), tc.config)
		}
		for rel, data := range tc.files {
			mkdirs(t, filepath.Join(repo, ".git"), filepath.Dir(rel))
			putFile(t, filepath.Join(repo, ".git", rel), data)
		}
		if tc.global != "" {
			putFile(t, filepath.Join(home, ".gitconfig"), tc.global)
		}
		if got := promisorRepo(repo); got != tc.want {
			t.Errorf("%s: promisor %v, want %v", name, got, tc.want)
		}
	}
	// A config that cannot be read as a plain file inside the repository
	// (a link leading out of .git) is taken as a possible partial clone.
	home := browseHome(t)
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "site")
	makeRepo(t, repo)
	outside := filepath.Join(shortTempDir(t), "config")
	putFile(t, outside, "[core]\n")
	symlink(t, outside, filepath.Join(repo, ".git", "config"))
	if !promisorRepo(repo) {
		t.Error("a config link leading out of .git was trusted")
	}
}
