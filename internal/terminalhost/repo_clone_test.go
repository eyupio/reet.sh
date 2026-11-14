//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// cloneFixture is a home with a bare repository standing in for
// github.com/o/r. Its GitRunner rewrites https://github.com/o/r.git to that
// repository with url.<path>.insteadOf, so a real clone runs without reaching
// the network; `git config --get` is not rewritten, so a clone still reads as
// one of github.com.
type cloneFixture struct {
	home, remote string
	// name is the repository under o the stand-in answers for; "r" unless
	// a test sets it.
	name string
	// onClone, when set, runs in place of the real clone: it is given the
	// context, the clone's arguments and the real runner.
	onClone func(ctx context.Context, dir string, args []string, real GitRunner) ([]byte, error)

	mu       sync.Mutex
	calls    [][]string
	cloneEnv []string
	ghCalls  [][]string
}

func newCloneFixture(t *testing.T) *cloneFixture {
	t.Helper()
	home := worktreeHome(t)
	work := newRepo(t, home, "work")
	remote := filepath.Join(home, "github.git")
	testGit(t, home, "clone", "-q", "--bare", work, remote)
	return &cloneFixture{home: home, remote: remote, name: "r"}
}

func (f *cloneFixture) git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, slices.Clone(args))
	isClone := slices.Contains(args, "clone")
	if isClone {
		f.cloneEnv = pinnedGitEnv(ctx)
	}
	f.mu.Unlock()
	real := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		// Only the clone is sent to the stand-in: every other call must see
		// the remote as the person's own configuration gives it.
		if !slices.Contains(args, "clone") {
			return ExecGit(ctx, dir, args...)
		}
		rewrite := []string{"-c", "url." + f.remote + ".insteadOf=https://github.com/o/" + f.name + ".git"}
		return ExecGit(ctx, dir, append(rewrite, args...)...)
	}
	if isClone && f.onClone != nil {
		return f.onClone(ctx, dir, args, real)
	}
	return real(ctx, dir, args...)
}

// signedIn is a GHRunner whose `gh auth status` succeeds.
func (f *cloneFixture) signedIn(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ghCalls = append(f.ghCalls, slices.Clone(args))
	return []byte("Logged in to github.com\n"), nil
}

func (f *cloneFixture) clone(t *testing.T, owner, repo string) terminalproto.RepoCloneResult {
	t.Helper()
	return f.cloneCtx(t, t.Context(), owner, repo)
}

func (f *cloneFixture) cloneCtx(t *testing.T, ctx context.Context, owner, repo string) terminalproto.RepoCloneResult {
	t.Helper()
	r := CloneRepo(ctx, f.home, terminalproto.RepoClone{Owner: owner, Repo: repo}, f.git, f.signedIn)
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: testRequestID(1), Generation: 1, Body: r}); err != nil {
		t.Fatalf("result is not a valid message: %+v", r)
	}
	return r
}

// cloneCalls is the recorded calls that were a clone.
func (f *cloneFixture) cloneCalls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, args := range f.calls {
		if slices.Contains(args, "clone") {
			out = append(out, args)
		}
	}
	return out
}

func (f *cloneFixture) repos() string { return filepath.Join(f.home, "reet", "repos") }

// handClone makes a clone of the stand-in at dir whose origin reads url, as a
// person would have made it.
func (f *cloneFixture) handClone(t *testing.T, dir, url string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	testGit(t, f.home, "clone", "-q", f.remote, dir)
	testGit(t, dir, "remote", "set-url", "origin", url)
}

func wantReason(t *testing.T, r terminalproto.RepoCloneResult, reason terminalproto.RepoCloneReason) {
	t.Helper()
	if r != (terminalproto.RepoCloneResult{Reason: reason}) {
		t.Fatalf("result = %+v, want reason %s", r, reason)
	}
}

func wantMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s is there (%v), want nothing", path, err)
	}
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestCloneRepoMakesAMarkedNormalClone(t *testing.T) {
	f := newCloneFixture(t)
	r := f.clone(t, "o", "r")
	dest := filepath.Join(f.repos(), "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
	if info, err := os.Lstat(filepath.Join(dest, ".git")); err != nil || !info.IsDir() {
		t.Fatalf(".git is not a directory: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, ".git", cloneMarker)); err != nil || string(data) != "o/r\n" {
		t.Fatalf("marker = %q, %v", data, err)
	}
	if info, _ := os.Lstat(filepath.Join(dest, ".git", cloneMarker)); info.Mode().Perm() != 0o600 {
		t.Fatalf("marker mode %v", info.Mode())
	}
	if origin, ok := cloneMarked(dest); !ok || origin != "o/r" {
		t.Fatalf("cloneMarked = %q, %v", origin, ok)
	}
	if got := testGit(t, dest, "config", "--get", "remote.origin.url"); got != "https://github.com/o/r.git" {
		t.Fatalf("origin = %q", got)
	}
	if testGit(t, dest, "cat-file", "-t", "HEAD:README") != "blob" || testGit(t, dest, "rev-parse", "--is-bare-repository") != "false" {
		t.Fatal("not a normal clone with its files")
	}
	for _, dir := range []string{filepath.Join(f.home, "reet"), f.repos(), filepath.Join(f.repos(), "o")} {
		if info, err := os.Lstat(dir); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", dir, info.Mode(), err)
		}
	}
	if names := entries(t, filepath.Join(f.repos(), "o")); !slices.Equal(names, []string{"r"}) {
		t.Fatalf("owner folder holds %q", names)
	}
	calls := f.cloneCalls()
	if len(calls) != 1 {
		t.Fatalf("clone calls %q", calls)
	}
	args := calls[0]
	for _, banned := range []string{"--depth", "--filter", "--bare", "--mirror", "--shallow-since", "--single-branch", "--no-tags"} {
		for _, arg := range args {
			if strings.HasPrefix(arg, banned) {
				t.Fatalf("clone argv has %s: %q", banned, args)
			}
		}
	}
	want := append(append(networkGitArgs(), cloneFilterArgs...), "clone", "--origin", "origin", "https://github.com/o/r.git")
	if !slices.Equal(args[:len(args)-1], want) || !slices.Contains(args, "core.hooksPath=/dev/null") || !slices.Contains(args, "protocol.ssh.allow=never") {
		t.Fatalf("clone argv %q, want %q then a temp folder", args, want)
	}
	temp := args[len(args)-1]
	if filepath.Dir(temp) != filepath.Join(f.repos(), "o") || !strings.HasPrefix(filepath.Base(temp), ".r.cloning-") || len(filepath.Base(temp)) != len(".r.cloning-")+16 {
		t.Fatalf("temp folder %q", temp)
	}
	if !slices.Contains(f.cloneEnv, "GIT_TERMINAL_PROMPT=0") || !slices.Contains(f.cloneEnv, "LANG=C") {
		t.Fatalf("clone environment %q", f.cloneEnv)
	}
	if len(f.ghCalls) != 1 || !slices.Equal(f.ghCalls[0], []string{"auth", "status", "--hostname", "github.com"}) {
		t.Fatalf("gh calls %q", f.ghCalls)
	}
}

func TestCloneRepoRefusesWhenSignedOut(t *testing.T) {
	f := newCloneFixture(t)
	gh := func(context.Context, ...string) ([]byte, error) {
		return []byte("You are not logged into any GitHub hosts.\n"), errors.New("exit status 1")
	}
	r := CloneRepo(t.Context(), f.home, terminalproto.RepoClone{Owner: "o", Repo: "r"}, f.git, gh)
	wantReason(t, r, terminalproto.RepoCloneNotSignedIn)
	wantMissing(t, filepath.Join(f.home, "reet"))
	if len(f.calls) != 0 {
		t.Fatalf("git ran: %q", f.calls)
	}
}

func TestCloneRepoFailsWithoutGitOrGH(t *testing.T) {
	f := newCloneFixture(t)
	noGH := func(context.Context, ...string) ([]byte, error) { return nil, errGHMissing }
	wantReason(t, CloneRepo(t.Context(), f.home, terminalproto.RepoClone{Owner: "o", Repo: "r"}, f.git, noGH), terminalproto.RepoCloneFailed)
	wantMissing(t, filepath.Join(f.home, "reet"))
	noGit := func(context.Context, string, ...string) ([]byte, error) { return nil, errGitMissing }
	wantReason(t, CloneRepo(t.Context(), f.home, terminalproto.RepoClone{Owner: "o", Repo: "r"}, noGit, f.signedIn), terminalproto.RepoCloneFailed)
	wantMissing(t, filepath.Join(f.repos(), "o"))
	wantReason(t, CloneRepo(t.Context(), f.home, terminalproto.RepoClone{Owner: "o", Repo: "r"}, nil, f.signedIn), terminalproto.RepoCloneFailed)
	wantReason(t, CloneRepo(t.Context(), f.home, terminalproto.RepoClone{Owner: "o", Repo: "r"}, f.git, nil), terminalproto.RepoCloneFailed)
}

func TestCloneRepoRefusesAnInvalidRequest(t *testing.T) {
	f := newCloneFixture(t)
	for _, req := range []terminalproto.RepoClone{{Owner: "o", Repo: ".."}, {Owner: "-o", Repo: "r"}, {Owner: "o", Repo: "a/b"}, {Owner: "", Repo: "r"}} {
		wantReason(t, CloneRepo(t.Context(), f.home, req, f.git, f.signedIn), terminalproto.RepoCloneFailed)
	}
	wantMissing(t, filepath.Join(f.home, "reet"))
	if len(f.ghCalls) != 0 || len(f.calls) != 0 {
		t.Fatal("an invalid request ran gh or git")
	}
}

func TestCloneRepoAdoptsItsOwnEarlierClone(t *testing.T) {
	f := newCloneFixture(t)
	first := f.clone(t, "o", "r")
	if first.Reason != "" {
		t.Fatal(first)
	}
	f.calls = nil
	second := f.clone(t, "o", "r")
	if second != (terminalproto.RepoCloneResult{Dir: first.Dir, DefaultBranch: "main", Adopted: true, Marked: true}) {
		t.Fatalf("second = %+v", second)
	}
	if calls := f.cloneCalls(); len(calls) != 0 {
		t.Fatalf("second call cloned: %q", calls)
	}
}

func TestCloneRepoAdoptsAPersonsCloneUnmarked(t *testing.T) {
	f := newCloneFixture(t)
	dest := filepath.Join(f.repos(), "o", "r")
	// The origin's spelling differs only in case: GitHub names are not
	// case-sensitive.
	f.handClone(t, dest, "git@github.com:O/R.git")
	r := f.clone(t, "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Adopted: true}) {
		t.Fatalf("result = %+v", r)
	}
	wantMissing(t, filepath.Join(dest, ".git", cloneMarker))
	if calls := f.cloneCalls(); len(calls) != 0 {
		t.Fatalf("cloned: %q", calls)
	}
}

func TestCloneRepoRefusesAnotherFolder(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func(t *testing.T, f *cloneFixture, dest string)
	}{
		{"plain folder", func(t *testing.T, f *cloneFixture, dest string) {
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dest, "notes"), "mine\n")
		}},
		{"empty folder", func(t *testing.T, f *cloneFixture, dest string) {
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"clone of another origin", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://github.com/o/other.git")
		}},
		{"clone of another host", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://gitlab.com/o/r.git")
		}},
		{"clone without origin", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://github.com/o/r.git")
			testGit(t, dest, "remote", "remove", "origin")
		}},
		{".git is a file", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, filepath.Join(f.home, "elsewhere"), "https://github.com/o/r.git")
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dest, ".git"), "gitdir: "+filepath.Join(f.home, "elsewhere", ".git")+"\n")
		}},
		{".git is a symlink", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, filepath.Join(f.home, "elsewhere"), "https://github.com/o/r.git")
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(f.home, "elsewhere", ".git"), filepath.Join(dest, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
		{"the folder is a symlink", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, filepath.Join(f.home, "elsewhere"), "https://github.com/o/r.git")
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(f.home, "elsewhere"), dest); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCloneFixture(t)
			dest := filepath.Join(f.repos(), "o", "r")
			tc.make(t, f, dest)
			before := snapshot(t, filepath.Dir(dest))
			wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneExists)
			if after := snapshot(t, filepath.Dir(dest)); !slices.Equal(before, after) {
				t.Fatalf("folder changed:\n%q\n%q", before, after)
			}
			if calls := f.cloneCalls(); len(calls) != 0 {
				t.Fatalf("cloned: %q", calls)
			}
		})
	}
}

// snapshot lists every path below dir with its mode and, for a file, its
// content, without following symlinks.
func snapshot(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		line := path + " " + info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			line += " " + string(data)
		}
		out = append(out, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// failClone stands in for a clone that wrote files and then failed.
func failClone(output string) func(context.Context, string, []string, GitRunner) ([]byte, error) {
	return func(_ context.Context, _ string, args []string, _ GitRunner) ([]byte, error) {
		temp := args[len(args)-1]
		if err := os.MkdirAll(filepath.Join(temp, ".git", "objects"), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(temp, "half-written"), []byte("x"), 0o600); err != nil {
			return nil, err
		}
		return []byte(output), errors.New("exit status 128")
	}
}

func TestCloneRepoLeavesNothingAfterAFailure(t *testing.T) {
	f := newCloneFixture(t)
	f.onClone = failClone("fatal: early EOF\n")
	wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneFailed)
	// The owner folder was made by this call and is removed with the clone.
	wantMissing(t, filepath.Join(f.repos(), "o"))
	if names := entries(t, f.repos()); len(names) != 0 {
		t.Fatalf("repos folder holds %q", names)
	}

	// An owner folder that was there before is kept, with what it held.
	other := filepath.Join(f.repos(), "o", "other")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneFailed)
	if names := entries(t, filepath.Join(f.repos(), "o")); !slices.Equal(names, []string{"other"}) {
		t.Fatalf("owner folder holds %q", names)
	}
	// So is an empty one.
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneFailed)
	if names := entries(t, filepath.Join(f.repos(), "o")); len(names) != 0 {
		t.Fatalf("owner folder holds %q", names)
	}
}

func TestCloneRepoNoAccess(t *testing.T) {
	for _, output := range []string{
		"Cloning into '/x'...\nremote: Repository not found.\nfatal: repository 'https://github.com/o/r.git/' not found\n",
		"Cloning into '/x'...\nfatal: could not read Username for 'https://github.com': terminal prompts disabled\n",
	} {
		f := newCloneFixture(t)
		f.onClone = failClone(output)
		wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneNoAccess)
		wantMissing(t, filepath.Join(f.repos(), "o"))
	}
}

// A repository GitHub does not have, through the real git: the stand-in
// path does not exist, which git reports in its own words and is failed.
func TestCloneRepoRealFailureLeavesNothing(t *testing.T) {
	f := newCloneFixture(t)
	if err := os.RemoveAll(f.remote); err != nil {
		t.Fatal(err)
	}
	r := f.clone(t, "o", "r")
	if r.Reason != terminalproto.RepoCloneFailed {
		t.Fatalf("result = %+v", r)
	}
	wantMissing(t, filepath.Join(f.repos(), "o"))
}

func TestCloneRepoIgnoresALeftoverTempFolder(t *testing.T) {
	f := newCloneFixture(t)
	leftover := filepath.Join(f.repos(), "o", ".r.cloning-0123456789abcdef")
	f.handClone(t, leftover, "https://github.com/o/r.git")
	writeFile(t, filepath.Join(leftover, ".git", cloneMarker), "o/r\n")
	r := f.clone(t, "o", "r")
	dest := filepath.Join(f.repos(), "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
	if len(f.cloneCalls()) != 1 {
		t.Fatal("no fresh clone")
	}
	// The leftover is not this call's to remove.
	if _, err := os.Lstat(filepath.Join(leftover, ".git", cloneMarker)); err != nil {
		t.Fatal("leftover changed", err)
	}
}

func TestCloneRepoRefusesASymlinkedParent(t *testing.T) {
	for _, link := range []string{"reet", "reet/repos", "reet/repos/o"} {
		t.Run(link, func(t *testing.T) {
			f := newCloneFixture(t)
			target := filepath.Join(f.home, "target")
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(f.home, link)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneFailed)
			if names := entries(t, target); len(names) != 0 {
				t.Fatalf("made %q through the symlink", names)
			}
			if len(f.cloneCalls()) != 0 {
				t.Fatal("cloned")
			}
		})
	}
}

func TestCloneRepoTimesOut(t *testing.T) {
	f := newCloneFixture(t)
	f.onClone = func(ctx context.Context, _ string, args []string, _ GitRunner) ([]byte, error) {
		temp := args[len(args)-1]
		if err := os.MkdirAll(filepath.Join(temp, ".git"), 0o700); err != nil {
			return nil, err
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	wantReason(t, f.cloneCtx(t, ctx, "o", "r"), terminalproto.RepoCloneTimedOut)
	wantMissing(t, filepath.Join(f.repos(), "o"))
}

// Two spellings of one name must never share or shadow a folder, whatever
// the file system does with case.
func TestCloneRepoRefusesACaseTwin(t *testing.T) {
	for _, tc := range []struct{ existing, owner, repo string }{
		{"O", "o", "r"},
		{"o/R", "o", "r"},
		{"O/r", "o", "r"},
	} {
		t.Run(tc.existing, func(t *testing.T) {
			f := newCloneFixture(t)
			existing := filepath.Join(f.repos(), tc.existing)
			if err := os.MkdirAll(existing, 0o700); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, f.repos())
			wantReason(t, f.clone(t, tc.owner, tc.repo), terminalproto.RepoCloneFailed)
			if after := snapshot(t, f.repos()); !slices.Equal(before, after) {
				t.Fatalf("repos changed:\n%q\n%q", before, after)
			}
			if len(f.cloneCalls()) != 0 {
				t.Fatal("cloned")
			}
		})
	}
	// The same spelling is no twin.
	f := newCloneFixture(t)
	if err := os.MkdirAll(filepath.Join(f.repos(), "O", "other"), 0o700); err != nil {
		t.Fatal(err)
	}
	f.onClone = failClone("fatal: early EOF\n")
	wantReason(t, f.clone(t, "O", "R"), terminalproto.RepoCloneFailed)
	if len(f.cloneCalls()) != 1 {
		t.Fatal("a folder of the same spelling blocked the clone")
	}
}

// The destination appears while the clone runs: the clone is dropped and
// what is there is judged as any existing folder is, never replaced.
func TestCloneRepoRenameRace(t *testing.T) {
	for _, tc := range []struct {
		name    string
		appear  func(t *testing.T, f *cloneFixture, dest string)
		adopted bool
	}{
		{"empty folder", func(t *testing.T, f *cloneFixture, dest string) {
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Error(err)
			}
		}, false},
		{"plain folder", func(t *testing.T, f *cloneFixture, dest string) {
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Error(err)
			}
			writeFile(t, filepath.Join(dest, "notes"), "mine\n")
		}, false},
		{"clone of the same origin", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://github.com/o/r")
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCloneFixture(t)
			dest := filepath.Join(f.repos(), "o", "r")
			var before []string
			f.onClone = func(ctx context.Context, dir string, args []string, real GitRunner) ([]byte, error) {
				out, err := real(ctx, dir, args...)
				tc.appear(t, f, dest)
				before = snapshot(t, dest)
				return out, err
			}
			r := f.clone(t, "o", "r")
			if tc.adopted {
				if r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Adopted: true}) {
					t.Fatalf("result = %+v", r)
				}
			} else {
				wantReason(t, r, terminalproto.RepoCloneExists)
			}
			if after := snapshot(t, dest); !slices.Equal(before, after) {
				t.Fatalf("destination changed:\n%q\n%q", before, after)
			}
			if names := entries(t, filepath.Join(f.repos(), "o")); !slices.Equal(names, []string{"r"}) {
				t.Fatalf("owner folder holds %q", names)
			}
		})
	}
}

func TestCloneMarked(t *testing.T) {
	home := worktreeHome(t)
	dir := newRepo(t, home, "repo")
	marker := filepath.Join(dir, ".git", cloneMarker)
	if _, ok := cloneMarked(dir); ok {
		t.Fatal("unmarked folder answered")
	}
	for content, want := range map[string]string{
		"o/r\n":       "o/r",
		"O-x/r.s_t\n": "O-x/r.s_t",
		"o/r":         "",
		"o/r\n\n":     "",
		"o\n":         "",
		"o/../r\n":    "",
		"-o/r\n":      "",
		"o/r/x\n":     "",
		"":            "",
	} {
		writeFile(t, marker, content)
		origin, ok := cloneMarked(dir)
		if ok != (want != "") || origin != want {
			t.Errorf("marker %q: cloneMarked = %q, %v", content, origin, ok)
		}
	}
	// A symlink is not a marker, even to a file that would be one.
	writeFile(t, filepath.Join(home, "elsewhere"), "o/r\n")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "elsewhere"), marker); err != nil {
		t.Fatal(err)
	}
	if origin, ok := cloneMarked(dir); ok {
		t.Fatalf("a symlinked marker answered %q", origin)
	}
}

// An adopted clone whose marker is a symlink is adopted unmarked, and the
// symlink is left as it is.
func TestCloneRepoAdoptsWithASymlinkedMarkerUnmarked(t *testing.T) {
	f := newCloneFixture(t)
	dest := filepath.Join(f.repos(), "o", "r")
	f.handClone(t, dest, "https://github.com/o/r.git")
	writeFile(t, filepath.Join(f.home, "elsewhere"), "o/r\n")
	if err := os.Symlink(filepath.Join(f.home, "elsewhere"), filepath.Join(dest, ".git", cloneMarker)); err != nil {
		t.Fatal(err)
	}
	r := f.clone(t, "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Adopted: true}) {
		t.Fatalf("result = %+v", r)
	}
	if info, err := os.Lstat(filepath.Join(dest, ".git", cloneMarker)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("marker symlink changed", err)
	}
	// A marker naming another repository does not mark this one.
	if err := os.Remove(filepath.Join(dest, ".git", cloneMarker)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dest, ".git", cloneMarker), "o/other\n")
	if r := f.clone(t, "o", "r"); !r.Adopted || r.Marked {
		t.Fatalf("result = %+v", r)
	}
}

// Without origin/HEAD, main or master, the default branch is the branch the
// clone checked out.
func TestCloneRepoDefaultBranchFallsBackToTheCheckout(t *testing.T) {
	f := newCloneFixture(t)
	testGit(t, f.remote, "branch", "-m", "main", "trunk")
	inner := f.git
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if slices.Contains(args, "refs/remotes/origin/HEAD") {
			return nil, errors.New("exit status 1")
		}
		return inner(ctx, dir, args...)
	}
	r := CloneRepo(t.Context(), f.home, terminalproto.RepoClone{Owner: "o", Repo: "r"}, git, f.signedIn)
	if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(f.repos(), "o", "r"), DefaultBranch: "trunk", Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
}

// ExecGH runs gh by absolute path with only HOME, PATH and LANG, and keeps at
// most 4 KiB of what it prints.
func TestExecGH(t *testing.T) {
	bin := shortTempDir(t)
	script := "#!/bin/sh\nenv | sort\necho \"args:$*\"\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("GH_TOKEN", "secret")
	t.Setenv("GIT_DIR", "/nowhere")
	out, err := ExecGH(t.Context(), "auth", "status", "--hostname", "github.com")
	if err != nil {
		t.Fatal(err, string(out))
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, _, _ := strings.Cut(line, "=")
		switch {
		case strings.HasPrefix(line, "args:"):
			if line != "args:auth status --hostname github.com" {
				t.Fatalf("args %q", line)
			}
		case key == "HOME" || key == "PATH" || key == "LANG" || key == "PWD" || key == "SHLVL" || key == "_":
			// sh sets PWD, SHLVL and _ itself.
		default:
			t.Fatalf("gh saw %q", line)
		}
	}

	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nhead -c 10000 /dev/zero | tr '\\0' x\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := ExecGH(t.Context()); err != nil || len(out) != 4<<10 {
		t.Fatalf("output %d bytes, %v", len(out), err)
	}

	t.Setenv("PATH", shortTempDir(t))
	if _, err := ExecGH(t.Context(), "auth", "status"); !errors.Is(err, errGHMissing) {
		t.Fatalf("no gh: %v", err)
	}
}

// lfsPointer is the text git-lfs commits in place of a large file.
const lfsPointer = "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393\nsize 12345\n"

// The clone reet starts reaches github.com and nothing else, and runs no
// LFS filter on a tree nobody has looked at: a repository whose
// .lfsconfig names another host over ssh, cloned on a machine whose own
// git configuration installs an LFS filter, leaves the marked file as the
// pointer it is and never starts the filter or ssh.
func TestCloneRepoRunsNoLFSFilter(t *testing.T) {
	for _, tc := range []struct {
		name string
		// real uses git-lfs itself as the filter, when it is installed.
		real bool
	}{
		{"recording filter", false},
		{"git-lfs", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.real {
				if _, err := exec.LookPath("git-lfs"); err != nil {
					t.Skip("git-lfs is not installed; the recording filter stands in for it")
				}
			}
			f := newCloneFixture(t)
			work := filepath.Join(f.home, "work")
			writeFile(t, filepath.Join(work, ".gitattributes"), "big.bin filter=lfs diff=lfs merge=lfs -text\n")
			writeFile(t, filepath.Join(work, ".lfsconfig"), "[lfs]\n\turl = ssh://git@attacker.invalid/x\n")
			writeFile(t, filepath.Join(work, "big.bin"), lfsPointer)
			testGit(t, work, "add", ".gitattributes", ".lfsconfig", "big.bin")
			testGit(t, work, "commit", "-q", "-m", "lfs")
			testGit(t, work, "push", "-q", f.remote, "main")

			// Only now does this HOME's configuration name the filter, so
			// the setup above ran none.
			bin := shortTempDir(t)
			record := filepath.Join(bin, "ran")
			recorder := func(name string) string {
				path := filepath.Join(bin, name)
				script := "#!/bin/sh\necho " + name + " >> '" + record + "'\nexit 1\n"
				if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
				return path
			}
			ssh := recorder("ssh")
			process, smudge := recorder("filter-process"), recorder("filter-smudge")+" %f"
			if tc.real {
				process, smudge = "git-lfs filter-process", "git-lfs smudge -- %f"
			}
			writeFile(t, filepath.Join(f.home, ".gitconfig"),
				"[filter \"lfs\"]\n\tprocess = "+process+"\n\tsmudge = "+smudge+"\n\tclean = "+recorder("filter-clean")+" %f\n\trequired = true\n"+
					"[core]\n\tsshCommand = "+ssh+"\n")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

			r := f.clone(t, "o", "r")
			dest := filepath.Join(f.repos(), "o", "r")
			if data, err := os.ReadFile(record); err == nil || !os.IsNotExist(err) {
				t.Fatalf("ran during the clone: %q (%v); result %+v", data, err, r)
			}
			if r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Marked: true}) {
				t.Fatalf("result = %+v", r)
			}
			if data, err := os.ReadFile(filepath.Join(dest, "big.bin")); err != nil || string(data) != lfsPointer {
				t.Fatalf("big.bin = %q, %v", data, err)
			}
			for _, args := range f.cloneCalls() {
				for _, want := range []string{"filter.lfs.process=", "filter.lfs.smudge=", "filter.lfs.required=false"} {
					if !slices.Contains(args, want) {
						t.Fatalf("clone argv %q lacks %s", args, want)
					}
				}
			}
			if !slices.Contains(f.cloneEnv, "GIT_LFS_SKIP_SMUDGE=1") {
				t.Fatalf("clone environment %q", f.cloneEnv)
			}
		})
	}
}

// The clone fetches tags as any clone does, and keeps them coming: no
// remote.origin.tagopt is written. Its remote is origin whatever the
// person's clone.defaultRemoteName says, since adoption and every later
// fetch name origin.
func TestCloneRepoKeepsTagsAndNamesOrigin(t *testing.T) {
	f := newCloneFixture(t)
	testGit(t, f.remote, "tag", "v1.0.0", "main")
	writeFile(t, filepath.Join(f.home, ".gitconfig"), "[clone]\n\tdefaultRemoteName = upstream\n")
	r := f.clone(t, "o", "r")
	dest := filepath.Join(f.repos(), "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
	if got := testGit(t, dest, "tag", "--list"); got != "v1.0.0" {
		t.Fatalf("tags %q", got)
	}
	if out, err := testGitErr(dest, "config", "--get", "remote.origin.tagopt"); err == nil {
		t.Fatalf("remote.origin.tagopt is set: %q", out)
	}
	if got := testGit(t, dest, "remote"); got != "origin" {
		t.Fatalf("remotes %q", got)
	}
	// A clone made so is adopted on a second request, which reads origin.
	if again := f.clone(t, "o", "r"); again != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Adopted: true, Marked: true}) {
		t.Fatalf("second = %+v", again)
	}
}

// A person's git settings that send github.com over ssh stop the clone,
// since ssh is refused; the answer says so rather than a bare failure.
func TestCloneRepoSSHRewrite(t *testing.T) {
	f := newCloneFixture(t)
	f.onClone = failClone("Cloning into '/x'...\nfatal: transport 'ssh' not allowed\n")
	wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneSSHRewrite)
	wantMissing(t, filepath.Join(f.repos(), "o"))

	// The real git, with the rewrite a developer commonly sets: the clone
	// runs without the stand-in's rewrite, so github.com is what git sees,
	// and git refuses ssh before it reaches any network.
	f = newCloneFixture(t)
	writeFile(t, filepath.Join(f.home, ".gitconfig"), "[url \"git@github.com:\"]\n\tinsteadOf = https://github.com/\n")
	f.onClone = func(ctx context.Context, dir string, args []string, _ GitRunner) ([]byte, error) {
		return ExecGit(ctx, dir, args...)
	}
	wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneSSHRewrite)
	wantMissing(t, filepath.Join(f.repos(), "o"))
}

// An empty repository clones, and has no default branch: its HEAD names a
// branch that has no commit.
func TestCloneRepoEmptyRepositoryHasNoDefaultBranch(t *testing.T) {
	f := newCloneFixture(t)
	if err := os.RemoveAll(f.remote); err != nil {
		t.Fatal(err)
	}
	testGit(t, f.home, "init", "-q", "--bare", "-b", "main", f.remote)
	r := f.clone(t, "o", "r")
	dest := filepath.Join(f.repos(), "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: dest, Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
	if again := f.clone(t, "o", "r"); again != (terminalproto.RepoCloneResult{Dir: dest, Adopted: true, Marked: true}) {
		t.Fatalf("adopted = %+v", again)
	}
}

// leftoverName is a temporary folder's name for repo, as CloneRepo makes it.
func leftoverName(repo string) string { return "." + repo + ".cloning-0123456789abcdef" }

// makeLeftover makes a folder at path with a half-written clone in it, last
// changed age ago.
func makeLeftover(t *testing.T, path string, age time.Duration) {
	t.Helper()
	makeLeftoverOf(t, path, "https://github.com/o/r.git", age)
}

// makeLeftoverOf makes what a clone of origin cut off part-way leaves at path:
// a repository with that origin and no commits, and a half-written file,
// last changed age ago. With origin "" it never got as far as .git.
func makeLeftoverOf(t *testing.T, path, origin string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		testGit(t, path, "init", "-q")
		testGit(t, path, "config", "remote.origin.url", origin)
	}
	writeFile(t, filepath.Join(path, "half-written"), "x")
	agePath(t, path, age)
}

func agePath(t *testing.T, path string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

// A clone cut off before it finished leaves its temporary folder; the next
// clone of that repository removes it, and only under every condition.
func TestCloneRepoRemovesAStaleLeftover(t *testing.T) {
	f := newCloneFixture(t)
	owner := filepath.Join(f.repos(), "o")
	stale := filepath.Join(owner, leftoverName("r"))
	makeLeftover(t, stale, 20*time.Minute)
	// The temporary folder of a clone cut off after its marker was written
	// names the repository, not the folder: it is a leftover too.
	writeFile(t, filepath.Join(stale, ".git", cloneMarker), "o/r\n")
	agePath(t, stale, 20*time.Minute)
	// Another spelling of the suffix is a leftover of its own.
	other := filepath.Join(owner, ".r.cloning-fedcba9876543210")
	makeLeftover(t, other, time.Hour)
	r := f.clone(t, "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(owner, "r"), DefaultBranch: "main", Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
	if names := entries(t, owner); !slices.Equal(names, []string{"r"}) {
		t.Fatalf("owner folder holds %q", names)
	}
}

func TestCloneRepoKeepsWhatIsNotALeftover(t *testing.T) {
	for _, tc := range []struct {
		name string
		// make builds what must be kept; keep lists the paths to check.
		make func(t *testing.T, f *cloneFixture, owner string) []string
		// repo is the repository cloned; "r" when empty.
		repo string
	}{
		{"younger than 15 minutes", func(t *testing.T, f *cloneFixture, owner string) []string {
			path := filepath.Join(owner, leftoverName("r"))
			makeLeftover(t, path, 14*time.Minute)
			return []string{path}
		}, ""},
		{"a symlink", func(t *testing.T, f *cloneFixture, owner string) []string {
			target := filepath.Join(f.home, "target")
			makeLeftover(t, target, time.Hour)
			if err := os.MkdirAll(owner, 0o700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(owner, leftoverName("r"))
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			return []string{link, target}
		}, ""},
		{"a file", func(t *testing.T, f *cloneFixture, owner string) []string {
			if err := os.MkdirAll(owner, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(owner, leftoverName("r"))
			writeFile(t, path, "mine\n")
			agePath(t, path, time.Hour)
			return []string{path}
		}, ""},
		{"a clone reet made of a repository with that name", func(t *testing.T, f *cloneFixture, owner string) []string {
			name := ".x.cloning-0123456789abcdef"
			path := filepath.Join(owner, name)
			f.handClone(t, path, "https://github.com/o/"+name+".git")
			writeFile(t, filepath.Join(path, ".git", cloneMarker), "o/"+name+"\n")
			agePath(t, path, time.Hour)
			return []string{path}
		}, "x"},
		{"another repository's leftover", func(t *testing.T, f *cloneFixture, owner string) []string {
			path := filepath.Join(owner, leftoverName("other"))
			makeLeftover(t, path, time.Hour)
			return []string{path}
		}, ""},
		{"another owner's leftover", func(t *testing.T, f *cloneFixture, owner string) []string {
			path := filepath.Join(f.repos(), "p", leftoverName("r"))
			makeLeftover(t, path, time.Hour)
			return []string{path}
		}, ""},
		{"a leftover below the owner folder", func(t *testing.T, f *cloneFixture, owner string) []string {
			path := filepath.Join(owner, "sub", leftoverName("r"))
			makeLeftover(t, path, time.Hour)
			return []string{path}
		}, ""},
		{"names that are not the temporary folder's", func(t *testing.T, f *cloneFixture, owner string) []string {
			var paths []string
			for _, name := range []string{
				".r.cloning-0123456789ABCDEF",
				".r.cloning-0123456789abcde",
				".r.cloning-0123456789abcdef0",
				".r.cloning-0123456789abcdeg",
				"r.cloning-0123456789abcdef",
				".R.cloning-0123456789abcdef",
				".r.cloning-0123456789abcdef.bak",
			} {
				path := filepath.Join(owner, name)
				makeLeftover(t, path, time.Hour)
				paths = append(paths, path)
			}
			return paths
		}, ""},
		{"owned by another user", func(t *testing.T, f *cloneFixture, owner string) []string {
			if os.Getuid() != 0 {
				t.Skip("only root can give a folder to another user")
			}
			path := filepath.Join(owner, leftoverName("r"))
			makeLeftover(t, path, time.Hour)
			if err := os.Chown(path, 4242, 4242); err != nil {
				t.Fatal(err)
			}
			agePath(t, path, time.Hour)
			return []string{path}
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCloneFixture(t)
			if tc.repo != "" {
				f.name = tc.repo
			}
			owner := filepath.Join(f.repos(), "o")
			keep := tc.make(t, f, owner)
			var before [][]string
			for _, path := range keep {
				before = append(before, snapshot(t, path))
			}
			r := f.clone(t, "o", f.name)
			if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(owner, f.name), DefaultBranch: "main", Marked: true}) {
				t.Fatalf("result = %+v", r)
			}
			for i, path := range keep {
				if after := snapshot(t, path); !slices.Equal(before[i], after) {
					t.Fatalf("%s changed:\n%q\n%q", path, before[i], after)
				}
			}
		})
	}
}

// A leftover that cannot be removed does not stop the clone.
func TestCloneRepoIgnoresALeftoverItCannotRemove(t *testing.T) {
	f := newCloneFixture(t)
	owner := filepath.Join(f.repos(), "o")
	stale := filepath.Join(owner, leftoverName("r"))
	makeLeftover(t, stale, time.Hour)
	var tried []string
	removeLeftover = func(path string) error {
		tried = append(tried, path)
		return errors.New("permission denied")
	}
	t.Cleanup(func() { removeLeftover = os.RemoveAll })
	r := f.clone(t, "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(owner, "r"), DefaultBranch: "main", Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
	if !slices.Equal(tried, []string{stale}) {
		t.Fatalf("tried to remove %q", tried)
	}
}

// A folder named like a leftover of o/x is removed only when its origin
// names o/x, or it has a .git with no origin set, or no .git at all. An
// origin that names anything else (the folder's own name, another
// repository, another host, something unreadable as a GitHub remote) and an
// origin that cannot be read keep it.
func TestCloneRepoLeftoverOrigin(t *testing.T) {
	const name = ".x.cloning-0123456789abcdef"
	for _, tc := range []struct {
		test string
		make func(t *testing.T, f *cloneFixture, path string)
		kept bool
	}{
		{"a person's clone of the repository of that name", func(t *testing.T, f *cloneFixture, path string) {
			f.handClone(t, path, "https://github.com/o/"+name)
		}, true},
		{"the same with its origin in another case and with .git", func(t *testing.T, f *cloneFixture, path string) {
			f.handClone(t, path, "https://github.com/O/.X.cloning-0123456789ABCDEF.git")
		}, true},
		{"the same over ssh", func(t *testing.T, f *cloneFixture, path string) {
			f.handClone(t, path, "git@github.com:o/"+name+".git")
		}, true},
		{"a .git whose config cannot be read", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
			writeFile(t, filepath.Join(path, ".git", "config"), "[remote \"origin\"\n\turl = = =\n")
		}, true},
		{"another repository", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "https://github.com/o/other.git", time.Hour)
		}, true},
		{"another host", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "https://gitlab.com/o/x.git", time.Hour)
		}, true},
		{"an origin that is not a GitHub remote", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "not a remote at all", time.Hour)
		}, true},
		{"a .git without an origin", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
			testGit(t, path, "config", "--unset", "remote.origin.url")
		}, true},
		{"an origin naming x in another case", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "https://github.com/O/X", time.Hour)
		}, false},
		{"a leftover with a marker", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
			writeFile(t, filepath.Join(path, ".git", cloneMarker), "o/x\n")
		}, false},
		{"a leftover without a marker", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
		}, false},
		{"a leftover that never got as far as .git", func(t *testing.T, f *cloneFixture, path string) {
			makeLeftoverOf(t, path, "", time.Hour)
		}, false},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			f.name = "x"
			owner := filepath.Join(f.repos(), "o")
			path := filepath.Join(owner, name)
			tc.make(t, f, path)
			agePath(t, path, time.Hour)
			before := snapshot(t, path)
			r := f.clone(t, "o", "x")
			if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(owner, "x"), DefaultBranch: "main", Marked: true}) {
				t.Fatalf("result = %+v", r)
			}
			if !tc.kept {
				wantMissing(t, path)
				return
			}
			if after := snapshot(t, path); !slices.Equal(before, after) {
				t.Fatalf("%s changed:\n%q\n%q", path, before, after)
			}
		})
	}
}

// A git that cannot read a candidate's origin (it fails, or runs out of
// time) keeps the folder.
func TestCloneRepoKeepsALeftoverWhoseOriginGitCannotRead(t *testing.T) {
	f := newCloneFixture(t)
	owner := filepath.Join(f.repos(), "o")
	stale := filepath.Join(owner, leftoverName("r"))
	makeLeftover(t, stale, time.Hour)
	var asked [][]string
	leftoverGit = func(_ context.Context, dir string, args ...string) ([]byte, error) {
		asked = append(asked, append([]string{dir}, args...))
		return nil, errors.New("git timed out: context deadline exceeded")
	}
	t.Cleanup(func() { leftoverGit = ExecGit })
	before := snapshot(t, stale)
	r := f.clone(t, "o", "r")
	if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(owner, "r"), DefaultBranch: "main", Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
	if after := snapshot(t, stale); !slices.Equal(before, after) {
		t.Fatalf("leftover changed:\n%q\n%q", before, after)
	}
	if len(asked) != 1 || !slices.Equal(asked[0], []string{stale, "config", "--local", "--get-all", "remote.origin.url"}) {
		t.Fatalf("asked %q", asked)
	}
}

// The origin is read from the candidate's own config, and only from a .git
// that is a real directory git accepts: a .git it does not accept (refs or
// HEAD gone, empty), a .git file or a symlink keeps the folder, and an
// origin in the person's global config never answers for the candidate.
func TestCloneRepoLeftoverOriginIsItsOwn(t *testing.T) {
	const name = ".x.cloning-0123456789abcdef"
	for _, tc := range []struct {
		test   string
		global string
		// make builds the candidate at path; it answers other paths that must
		// be left untouched too.
		make func(t *testing.T, f *cloneFixture, path string) []string
		kept bool
	}{
		{"refs removed", "", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "https://github.com/o/other", time.Hour)
			if err := os.RemoveAll(filepath.Join(path, ".git", "refs")); err != nil {
				t.Fatal(err)
			}
			return nil
		}, true},
		{"HEAD removed", "", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "https://github.com/o/other", time.Hour)
			if err := os.Remove(filepath.Join(path, ".git", "HEAD")); err != nil {
				t.Fatal(err)
			}
			return nil
		}, true},
		{"an empty .git", "", func(t *testing.T, f *cloneFixture, path string) []string {
			if err := os.MkdirAll(filepath.Join(path, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			return nil
		}, true},
		{"a .git file", "", func(t *testing.T, f *cloneFixture, path string) []string {
			target := filepath.Join(f.home, "elsewhere")
			f.handClone(t, target, "https://github.com/o/x.git")
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(path, ".git"), "gitdir: "+filepath.Join(target, ".git")+"\n")
			return []string{target}
		}, true},
		{"a .git symlink", "", func(t *testing.T, f *cloneFixture, path string) []string {
			target := filepath.Join(f.home, "elsewhere")
			f.handClone(t, target, "https://github.com/o/x.git")
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(target, ".git"), filepath.Join(path, ".git")); err != nil {
				t.Fatal(err)
			}
			return []string{target}
		}, true},
		{"no origin of its own, another one globally", "https://github.com/o/other", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "", time.Hour)
			testGit(t, path, "init", "-q")
			return nil
		}, true},
		{"no origin of its own, this one globally", "https://github.com/o/x", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "", time.Hour)
			testGit(t, path, "init", "-q")
			return nil
		}, true},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			f.name = "x"
			owner := filepath.Join(f.repos(), "o")
			path := filepath.Join(owner, name)
			others := tc.make(t, f, path)
			agePath(t, path, time.Hour)
			if tc.global != "" {
				// Set while the leftovers are judged; the clone itself, which
				// comes after, runs without it, as a global origin would send
				// the stand-in's fetch elsewhere.
				global := filepath.Join(f.home, ".gitconfig")
				writeFile(t, global, "[remote \"origin\"]\n\turl = "+tc.global+"\n")
				f.onClone = func(ctx context.Context, dir string, args []string, real GitRunner) ([]byte, error) {
					if err := os.Remove(global); err != nil {
						return nil, err
					}
					return real(ctx, dir, args...)
				}
			}
			var asked [][]string
			leftoverGit = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
				asked = append(asked, slices.Clone(args))
				return ExecGit(ctx, dir, args...)
			}
			t.Cleanup(func() { leftoverGit = ExecGit })
			var before [][]string
			for _, p := range append([]string{path}, others...) {
				before = append(before, snapshot(t, p))
			}
			r := f.clone(t, "o", "x")
			if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(owner, "x"), DefaultBranch: "main", Marked: true}) {
				t.Fatalf("result = %+v", r)
			}
			for i, p := range append([]string{path}, others...) {
				if i == 0 && !tc.kept {
					wantMissing(t, path)
					continue
				}
				if after := snapshot(t, p); !slices.Equal(before[i], after) {
					t.Fatalf("%s changed:\n%q\n%q", p, before[i], after)
				}
			}
			for _, args := range asked {
				if !slices.Equal(args, []string{"config", "--local", "--get-all", "remote.origin.url"}) {
					t.Fatalf("asked %q", args)
				}
			}
			// A candidate git accepts as a repository is asked once (with
			// --local, above); one it does not, or whose .git is not a real
			// directory, may not be asked at all.
			if tc.global != "" && len(asked) != 1 {
				t.Fatalf("asked %q", asked)
			}
		})
	}
}

// Adoption reads the destination's own origin: an origin in the person's
// global config never makes a folder a clone of the repository.
func TestCloneRepoAdoptsOnlyItsOwnOrigin(t *testing.T) {
	f := newCloneFixture(t)
	dest := filepath.Join(f.repos(), "o", "r")
	if err := os.MkdirAll(filepath.Join(dest, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.home, ".gitconfig"), "[remote \"origin\"]\n\turl = https://github.com/o/r\n")
	before := snapshot(t, dest)
	wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneExists)
	if after := snapshot(t, dest); !slices.Equal(before, after) {
		t.Fatalf("destination changed:\n%q\n%q", before, after)
	}

	// A real clone there is still adopted, with the global origin set too.
	if err := os.RemoveAll(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.home, ".gitconfig")); err != nil {
		t.Fatal(err)
	}
	f.handClone(t, dest, "https://github.com/o/r.git")
	writeFile(t, filepath.Join(f.home, ".gitconfig"), "[remote \"origin\"]\n\turl = https://github.com/o/r\n")
	if r := f.clone(t, "o", "r"); r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Adopted: true}) {
		t.Fatalf("result = %+v", r)
	}
	for _, args := range f.calls {
		if slices.Contains(args, "remote.origin.url") && !slices.Equal(args[len(args)-4:], []string{"config", "--local", "--get-all", "remote.origin.url"}) {
			t.Fatalf("origin read %q", args)
		}
	}
}

// A leftover is removed only on proof it is this repository's: no .git at
// all, or a .git with a plain config of its own whose origin names o/x. Every
// way the origin could be set where `git config --local` does not see it, or
// see it alone, keeps the folder.
func TestCloneRepoRemovesALeftoverOnlyOnProof(t *testing.T) {
	const name = ".x.cloning-0123456789abcdef"
	for _, tc := range []struct {
		test string
		make func(t *testing.T, f *cloneFixture, path string) []string
		kept bool
	}{
		{"a valid .git with no origin", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "", time.Hour)
			testGit(t, path, "init", "-q")
			return nil
		}, true},
		{"no .git", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "", time.Hour)
			return nil
		}, false},
		{"a clone-shaped repository with origin o/x", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
			return nil
		}, false},
		{"origin o/other", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "https://github.com/o/other.git", time.Hour)
			return nil
		}, true},
		{"config is a directory", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
			config := filepath.Join(path, ".git", "config")
			if err := os.Remove(config); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(config, 0o700); err != nil {
				t.Fatal(err)
			}
			return nil
		}, true},
		{"config is a dangling symlink", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
			config := filepath.Join(path, ".git", "config")
			if err := os.Remove(config); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(f.home, "nowhere"), config); err != nil {
				t.Fatal(err)
			}
			return nil
		}, true},
		{"config is a symlink to one naming o/x", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
			config := filepath.Join(path, ".git", "config")
			elsewhere := filepath.Join(f.home, "config-elsewhere")
			if err := os.Rename(config, elsewhere); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, config); err != nil {
				t.Fatal(err)
			}
			return []string{elsewhere}
		}, true},
		{"origin o/other, then o/x added", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "https://github.com/o/other.git", time.Hour)
			testGit(t, path, "config", "--add", "remote.origin.url", "https://github.com/o/x.git")
			return nil
		}, true},
		{"origin set only through include.path", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "", time.Hour)
			testGit(t, path, "init", "-q")
			included := filepath.Join(f.home, "included")
			writeFile(t, included, "[remote \"origin\"]\n\turl = https://github.com/o/other\n")
			testGit(t, path, "config", "include.path", included)
			return []string{included}
		}, true},
		{"origin in config.worktree", func(t *testing.T, f *cloneFixture, path string) []string {
			makeLeftoverOf(t, path, "", time.Hour)
			testGit(t, path, "init", "-q")
			testGit(t, path, "config", "extensions.worktreeConfig", "true")
			writeFile(t, filepath.Join(path, ".git", "config.worktree"), "[remote \"origin\"]\n\turl = https://github.com/o/other\n")
			return nil
		}, true},
		{"commondir pointing at a clone of o/x", func(t *testing.T, f *cloneFixture, path string) []string {
			target := filepath.Join(f.home, "elsewhere")
			f.handClone(t, target, "https://github.com/o/x.git")
			makeLeftoverOf(t, path, "", time.Hour)
			testGit(t, path, "init", "-q")
			writeFile(t, filepath.Join(path, ".git", "commondir"), filepath.Join(target, ".git")+"\n")
			return []string{target}
		}, true},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			f.name = "x"
			owner := filepath.Join(f.repos(), "o")
			path := filepath.Join(owner, name)
			others := tc.make(t, f, path)
			agePath(t, path, time.Hour)
			all := append([]string{path}, others...)
			var before [][]string
			for _, p := range all {
				before = append(before, snapshot(t, p))
			}
			r := f.clone(t, "o", "x")
			if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(owner, "x"), DefaultBranch: "main", Marked: true}) {
				t.Fatalf("result = %+v", r)
			}
			for i, p := range all {
				if i == 0 && !tc.kept {
					wantMissing(t, path)
					continue
				}
				if after := snapshot(t, p); !slices.Equal(before[i], after) {
					t.Fatalf("%s changed:\n%q\n%q", p, before[i], after)
				}
			}
		})
	}
}

// A destination whose .git shares another repository's config (commondir),
// or whose config is not a plain file of its own, is not adopted.
func TestCloneRepoAdoptsOnlyAPlainConfig(t *testing.T) {
	for _, tc := range []struct {
		test string
		make func(t *testing.T, f *cloneFixture, dest string)
	}{
		{"commondir", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, filepath.Join(f.home, "elsewhere"), "https://github.com/o/r.git")
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			testGit(t, dest, "init", "-q")
			writeFile(t, filepath.Join(dest, ".git", "commondir"), filepath.Join(f.home, "elsewhere", ".git")+"\n")
		}},
		{"config.worktree", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://github.com/o/r.git")
			writeFile(t, filepath.Join(dest, ".git", "config.worktree"), "")
		}},
		{"two origins, the last naming it", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://github.com/o/other.git")
			testGit(t, dest, "config", "--add", "remote.origin.url", "https://github.com/o/r.git")
		}},
		{"config a symlink", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://github.com/o/r.git")
			config := filepath.Join(dest, ".git", "config")
			elsewhere := filepath.Join(f.home, "config-elsewhere")
			if err := os.Rename(config, elsewhere); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, config); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dest := filepath.Join(f.repos(), "o", "r")
			tc.make(t, f, dest)
			before := snapshot(t, dest)
			wantReason(t, f.clone(t, "o", "r"), terminalproto.RepoCloneExists)
			if after := snapshot(t, dest); !slices.Equal(before, after) {
				t.Fatalf("destination changed:\n%q\n%q", before, after)
			}
		})
	}
	// A real clone is still adopted.
	f := newCloneFixture(t)
	dest := filepath.Join(f.repos(), "o", "r")
	f.handClone(t, dest, "https://github.com/o/r.git")
	if r := f.clone(t, "o", "r"); r != (terminalproto.RepoCloneResult{Dir: dest, DefaultBranch: "main", Adopted: true}) {
		t.Fatalf("result = %+v", r)
	}
}

// A leftover whose config this user cannot read is kept, though its origin
// names the repository being cloned: git's answer without the config is no
// proof. (Root reads a file of mode 000, so this runs only as another user.)
func TestCloneRepoKeepsALeftoverWithAnUnreadableConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file of mode 000")
	}
	f := newCloneFixture(t)
	f.name = "x"
	owner := filepath.Join(f.repos(), "o")
	path := filepath.Join(owner, ".x.cloning-0123456789abcdef")
	makeLeftoverOf(t, path, "https://github.com/o/x.git", time.Hour)
	config := filepath.Join(path, ".git", "config")
	info, err := os.Lstat(config)
	if err != nil {
		t.Fatal(err)
	}
	mode := info.Mode().Perm()
	before := snapshot(t, path)
	if err := os.Chmod(config, 0); err != nil {
		t.Fatal(err)
	}
	restore := func() { _ = os.Chmod(config, mode) }
	t.Cleanup(restore)
	r := f.clone(t, "o", "x")
	restore()
	if r != (terminalproto.RepoCloneResult{Dir: filepath.Join(owner, "x"), DefaultBranch: "main", Marked: true}) {
		t.Fatalf("result = %+v", r)
	}
	if after := snapshot(t, path); !slices.Equal(before, after) {
		t.Fatalf("%s changed:\n%q\n%q", path, before, after)
	}
}

// madeClone is a clone of o/r that CloneRepo made, at its folder.
func (f *cloneFixture) madeClone(t *testing.T) string {
	t.Helper()
	r := f.clone(t, "o", "r")
	if r.Reason != "" || !r.Marked || r.Adopted {
		t.Fatalf("clone = %+v", r)
	}
	return r.Dir
}

func (f *cloneFixture) status(t *testing.T, dir, origin string) terminalproto.RepoCloneState {
	t.Helper()
	return f.statusWith(t, dir, origin, f.git)
}

func (f *cloneFixture) statusWith(t *testing.T, dir, origin string, git GitRunner) terminalproto.RepoCloneState {
	t.Helper()
	s := CloneStatus(t.Context(), f.home, terminalproto.RepoCloneStatus{Dir: dir, Origin: origin}, git)
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: testRequestID(1), Generation: 1, Body: s}); err != nil {
		t.Fatalf("state is not a valid message: %+v", s)
	}
	return s
}

func (f *cloneFixture) remove(t *testing.T, dir, origin, fingerprint string) terminalproto.RepoCloneRemoved {
	t.Helper()
	return f.removeWith(t, dir, origin, fingerprint, f.git)
}

func (f *cloneFixture) removeWith(t *testing.T, dir, origin, fingerprint string, git GitRunner) terminalproto.RepoCloneRemoved {
	t.Helper()
	r := RemoveClone(t.Context(), f.home, terminalproto.RepoCloneRemove{Dir: dir, Origin: origin, Fingerprint: fingerprint}, git)
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: testRequestID(1), Generation: 1, Body: r}); err != nil {
		t.Fatalf("answer is not a valid message: %+v", r)
	}
	return r
}

// push pushes a branch of the clone at dir to the stand-in for github.com.
func (f *cloneFixture) push(t *testing.T, dir, branch string) {
	t.Helper()
	testGit(t, dir, "-c", "url."+f.remote+".insteadOf=https://github.com/o/"+f.name+".git", "push", "-q", "origin", branch)
}

// gitSub is the git subcommand of args, after any leading "-c name=value".
func gitSub(args []string) string {
	for len(args) >= 2 && args[0] == "-c" {
		args = args[2:]
	}
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// anyFingerprint stands for a fingerprint a person was never shown.
const anyFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// wantCounts checks a marked state's four counts and that it is not
// removable unless all are zero.
func wantCounts(t *testing.T, s terminalproto.RepoCloneState, worktrees, changed, stashes, unpushed int) {
	t.Helper()
	if s.Reason != "" || !s.Marked || len(s.Fingerprint) != 64 ||
		s.Worktrees != worktrees || s.Changed != changed || s.Stashes != stashes || s.Unpushed != unpushed {
		t.Fatalf("state = %+v, want worktrees %d changed %d stashes %d unpushed %d", s, worktrees, changed, stashes, unpushed)
	}
	if s.Removable() != (worktrees+changed+stashes+unpushed == 0) {
		t.Fatalf("Removable() = %v for %+v", s.Removable(), s)
	}
}

// wantKept checks that a remove was refused with reason and that nothing
// under root changed since before was taken.
func wantKept(t *testing.T, r terminalproto.RepoCloneRemoved, reason terminalproto.RepoCloneReason, root string, before []string) {
	t.Helper()
	if r != (terminalproto.RepoCloneRemoved{Reason: reason}) {
		t.Fatalf("remove = %+v, want reason %s", r, reason)
	}
	if after := snapshot(t, root); !slices.Equal(after, before) {
		t.Fatalf("a refused remove changed the folder:\nbefore %q\nafter  %q", before, after)
	}
}

func TestCloneStatusOfAFreshCloneIsRemovable(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	// A file whose time changed tempts git to rewrite its index.
	agePath(t, filepath.Join(dir, "README"), time.Hour)
	before := snapshot(t, f.home)
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 0, 0, 0)
	if again := f.status(t, dir, "o/r"); again != s {
		t.Fatalf("a second look differs: %+v, %+v", s, again)
	}
	if after := snapshot(t, f.home); !slices.Equal(after, before) {
		t.Fatalf("status changed the clone:\nbefore %q\nafter  %q", before, after)
	}
}

// A clone of a repository with no commits holds nothing either.
func TestCloneStatusOfAnEmptyRepositoryIsRemovable(t *testing.T) {
	f := newCloneFixture(t)
	if err := os.RemoveAll(f.remote); err != nil {
		t.Fatal(err)
	}
	testGit(t, f.home, "init", "-q", "--bare", "-b", "main", f.remote)
	dir := f.madeClone(t)
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 0, 0)
	writeFile(t, filepath.Join(dir, "new"), "x")
	if s := f.status(t, dir, "o/r"); s.Removable() {
		t.Fatalf("an untracked file in an empty clone is removable: %+v", s)
	}
}

func TestCloneStatusCountsALinkedWorktree(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	// The worktree's branch sits on a pushed commit: only the worktree counts.
	testGit(t, dir, "worktree", "add", "-q", "-b", "task", filepath.Join(f.home, "wt"), "origin/main")
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 1, 0, 0, 0)
	before := snapshot(t, f.home)
	wantKept(t, f.remove(t, dir, "o/r", s.Fingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
}

func TestCloneStatusCountsAnUntrackedFile(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	// An ignored file is not counted.
	writeFile(t, filepath.Join(dir, ".git", "info", "exclude"), "ignored\n")
	writeFile(t, filepath.Join(dir, "ignored"), "x")
	clean := f.status(t, dir, "o/r")
	wantCounts(t, clean, 0, 0, 0, 0)
	writeFile(t, filepath.Join(dir, "new"), "x")
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 1, 0, 0)
	writeFile(t, filepath.Join(dir, "README"), "edited\n")
	edited := f.status(t, dir, "o/r")
	wantCounts(t, edited, 0, 2, 0, 0)
	if s.Fingerprint == clean.Fingerprint || edited.Fingerprint == s.Fingerprint {
		t.Fatal("the fingerprint did not follow the changes")
	}
	before := snapshot(t, f.home)
	wantKept(t, f.remove(t, dir, "o/r", edited.Fingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
}

func TestCloneStatusCountsAStash(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	stash := func(text string) {
		writeFile(t, filepath.Join(dir, "README"), text)
		testGit(t, dir, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "stash", "-q")
	}
	stash("one\n")
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 0, 1, 0)
	stash("two\n")
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 2, 0)
	before := snapshot(t, f.home)
	wantKept(t, f.remove(t, dir, "o/r", s.Fingerprint), terminalproto.RepoCloneChanged, f.home, before)
}

func TestCloneStatusCountsAnUnpushedBranch(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "-b", "task")
	commitIn(t, dir, "a")
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 0, 0, 1)
	// A second branch on the same commit is a second branch that would go.
	testGit(t, dir, "branch", "twin")
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 0, 2)
	// main ahead of origin/main counts as well.
	testGit(t, dir, "checkout", "-q", "main")
	commitIn(t, dir, "b")
	s = f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 0, 0, 3)
	before := snapshot(t, f.home)
	wantKept(t, f.remove(t, dir, "o/r", s.Fingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
}

// A branch reet made for a run and pushed (its pull request may be open or
// merged) holds nothing that is not on GitHub.
func TestCloneStatusCountsAPushedReetBranchAsSafe(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "-b", "reet/run-1")
	commitIn(t, dir, "a")
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 0, 1)
	f.push(t, dir, "reet/run-1")
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 0, 0)
	// A branch behind what was pushed is safe too.
	testGit(t, dir, "branch", "behind", "origin/main")
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 0, 0)
	// Another remote does not count: only origin is known to be GitHub.
	testGit(t, dir, "checkout", "-q", "-b", "elsewhere")
	commitIn(t, dir, "b")
	testGit(t, dir, "remote", "add", "backup", f.remote)
	testGit(t, dir, "update-ref", "refs/remotes/backup/elsewhere", "HEAD")
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 0, 1)
}

func TestCloneStatusCountsADetachedUnpushedHead(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	testGit(t, dir, "checkout", "-q", "--detach")
	detached := f.status(t, dir, "o/r")
	wantCounts(t, detached, 0, 0, 0, 0)
	commitIn(t, dir, "a")
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 0, 0, 1)
	if s.Fingerprint == detached.Fingerprint {
		t.Fatal("the fingerprint did not follow HEAD")
	}
	before := snapshot(t, f.home)
	wantKept(t, f.remove(t, dir, "o/r", s.Fingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
}

func TestCloneStatusFingerprintFollowsWhatChanges(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	seen := map[string]string{}
	look := func(what string) {
		t.Helper()
		s := f.status(t, dir, "o/r")
		if s.Reason != "" || len(s.Fingerprint) != 64 {
			t.Fatalf("%s: %+v", what, s)
		}
		if earlier, ok := seen[s.Fingerprint]; ok {
			t.Fatalf("%s has the fingerprint of %s", what, earlier)
		}
		seen[s.Fingerprint] = what
	}
	look("the fresh clone")
	testGit(t, dir, "branch", "task")
	look("a new branch")
	testGit(t, dir, "checkout", "-q", "task")
	look("HEAD on another branch")
	commitIn(t, dir, "a")
	look("a branch that moved")
	testGit(t, dir, "tag", "v1")
	look("a new tag")
	writeFile(t, filepath.Join(dir, "README"), "one\n")
	look("an edited file")
	writeFile(t, filepath.Join(dir, "README"), "two\n")
	look("the same file edited again")
	testGit(t, dir, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "stash", "-q")
	look("a stash")
	writeFile(t, filepath.Join(dir, "README"), "three\n")
	testGit(t, dir, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "stash", "-q")
	look("a second stash")
	testGit(t, dir, "stash", "drop", "-q", "stash@{1}")
	look("an older stash dropped")
	testGit(t, dir, "worktree", "add", "-q", "--detach", filepath.Join(f.home, "wt"))
	look("a linked worktree")
}

// notAPlainClone are folders at the clone's path that are not provably a
// plain clone of o/r: each starts from a clone reet made.
var notAPlainClone = []struct {
	test string
	make func(t *testing.T, f *cloneFixture, dir string)
}{
	{".git is a file", func(t *testing.T, f *cloneFixture, dir string) {
		moved := filepath.Join(f.home, "moved.git")
		if err := os.Rename(filepath.Join(dir, ".git"), moved); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, ".git"), "gitdir: "+moved+"\n")
	}},
	{".git is a symlink", func(t *testing.T, f *cloneFixture, dir string) {
		moved := filepath.Join(f.home, "moved.git")
		if err := os.Rename(filepath.Join(dir, ".git"), moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(moved, filepath.Join(dir, ".git")); err != nil {
			t.Fatal(err)
		}
	}},
	{"no .git", func(t *testing.T, f *cloneFixture, dir string) {
		if err := os.Rename(filepath.Join(dir, ".git"), filepath.Join(f.home, "moved.git")); err != nil {
			t.Fatal(err)
		}
	}},
	{"commondir", func(t *testing.T, f *cloneFixture, dir string) {
		f.handClone(t, filepath.Join(f.home, "elsewhere"), "https://github.com/o/r.git")
		writeFile(t, filepath.Join(dir, ".git", "commondir"), filepath.Join(f.home, "elsewhere", ".git")+"\n")
	}},
	{"config.worktree", func(t *testing.T, f *cloneFixture, dir string) {
		writeFile(t, filepath.Join(dir, ".git", "config.worktree"), "")
	}},
	{"config is a symlink", func(t *testing.T, f *cloneFixture, dir string) {
		config := filepath.Join(dir, ".git", "config")
		if err := os.Rename(config, filepath.Join(f.home, "config")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(f.home, "config"), config); err != nil {
			t.Fatal(err)
		}
	}},
	{"two origins", func(t *testing.T, f *cloneFixture, dir string) {
		testGit(t, dir, "config", "--add", "remote.origin.url", "https://github.com/o/r.git")
	}},
	{"two origins, the first another repository", func(t *testing.T, f *cloneFixture, dir string) {
		testGit(t, dir, "remote", "set-url", "origin", "https://github.com/o/other.git")
		testGit(t, dir, "config", "--add", "remote.origin.url", "https://github.com/o/r.git")
	}},
	{"another origin", func(t *testing.T, f *cloneFixture, dir string) {
		testGit(t, dir, "remote", "set-url", "origin", "https://github.com/o/other.git")
	}},
	{"an origin on another host", func(t *testing.T, f *cloneFixture, dir string) {
		testGit(t, dir, "remote", "set-url", "origin", "https://gitlab.com/o/r.git")
	}},
	{"no origin", func(t *testing.T, f *cloneFixture, dir string) {
		testGit(t, dir, "remote", "remove", "origin")
	}},
}

// Anything at the clone's path that is not provably a plain clone of the
// repository is reported as something else, and never removed.
func TestRemoveCloneRefusesAnotherOrigin(t *testing.T) {
	for _, tc := range notAPlainClone {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			tc.make(t, f, dir)
			before := snapshot(t, f.home)
			s := f.status(t, dir, "o/r")
			if s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneExists}) || s.Removable() {
				t.Fatalf("state = %+v", s)
			}
			wantKept(t, f.remove(t, dir, "o/r", anyFingerprint), terminalproto.RepoCloneExists, f.home, before)
		})
	}
}

// A folder without the marker reet writes, or with one that names another
// repository, is a person's own: nothing is counted and it is never removed.
func TestRemoveCloneRefusesAnUnmarkedFolder(t *testing.T) {
	for _, tc := range []struct {
		test string
		make func(t *testing.T, f *cloneFixture, dir string)
	}{
		{"a clone a person made", func(t *testing.T, f *cloneFixture, dir string) {
			f.handClone(t, dir, "https://github.com/o/r.git")
		}},
		{"a marker naming another repository", func(t *testing.T, f *cloneFixture, dir string) {
			f.handClone(t, dir, "https://github.com/o/r.git")
			writeFile(t, filepath.Join(dir, ".git", cloneMarker), "o/other\n")
		}},
		{"a marker that is a symlink", func(t *testing.T, f *cloneFixture, dir string) {
			f.handClone(t, dir, "https://github.com/o/r.git")
			writeFile(t, filepath.Join(f.home, "marker"), "o/r\n")
			if err := os.Symlink(filepath.Join(f.home, "marker"), filepath.Join(dir, ".git", cloneMarker)); err != nil {
				t.Fatal(err)
			}
		}},
		{"a marker that is not one line", func(t *testing.T, f *cloneFixture, dir string) {
			f.handClone(t, dir, "https://github.com/o/r.git")
			writeFile(t, filepath.Join(dir, ".git", cloneMarker), "o/r")
		}},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := filepath.Join(f.repos(), "o", "r")
			tc.make(t, f, dir)
			before := snapshot(t, f.home)
			s := f.status(t, dir, "o/r")
			if s != (terminalproto.RepoCloneState{}) || s.Removable() {
				t.Fatalf("state = %+v, want an unmarked one", s)
			}
			wantKept(t, f.remove(t, dir, "o/r", anyFingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
		})
	}
}

// Only <home>/reet/repos/<owner>/<repo> for the origin asked about is ever
// looked at, and only when every folder down to it is a real one.
func TestRemoveCloneRefusesAPathOutsideReetRepos(t *testing.T) {
	for _, tc := range []struct {
		test string
		// make returns the Dir and Origin to ask about.
		make func(t *testing.T, f *cloneFixture, dir string) (string, string)
	}{
		{"a clone elsewhere under home", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			work := filepath.Join(f.home, "work", "r")
			f.handClone(t, work, "https://github.com/o/r.git")
			writeFile(t, filepath.Join(work, ".git", cloneMarker), "o/r\n")
			return work, "o/r"
		}},
		{"a symlink into reet/repos", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			link := filepath.Join(f.home, "link")
			if err := os.Symlink(dir, link); err != nil {
				t.Fatal(err)
			}
			return link, "o/r"
		}},
		{"another origin's folder", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			return dir, "o/other"
		}},
		{"another owner", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			return dir, "p/r"
		}},
		{"another spelling of the origin", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			return dir, "O/R"
		}},
		{"a trailing slash", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			return dir + "/", "o/r"
		}},
		{"a path that climbs back", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			return filepath.Join(f.repos(), "o") + "/x/../r", "o/r"
		}},
		{"the owner folder", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			return filepath.Dir(dir), "o/r"
		}},
		{"an origin that is not one", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			return dir, "o/r/x"
		}},
		{"nothing there", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			return filepath.Join(f.repos(), "o", "gone"), "o/gone"
		}},
		{"the clone's folder is a symlink", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			moved := filepath.Join(f.home, "moved")
			if err := os.Rename(dir, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(moved, dir); err != nil {
				t.Fatal(err)
			}
			return dir, "o/r"
		}},
		{"the owner folder is a symlink", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			owner, moved := filepath.Dir(dir), filepath.Join(f.home, "moved")
			if err := os.Rename(owner, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(moved, owner); err != nil {
				t.Fatal(err)
			}
			return dir, "o/r"
		}},
		{"reet/repos is a symlink", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			moved := filepath.Join(f.home, "moved")
			if err := os.Rename(f.repos(), moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(moved, f.repos()); err != nil {
				t.Fatal(err)
			}
			return dir, "o/r"
		}},
		{"the owner folder is open to others", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			if err := os.Chmod(filepath.Dir(dir), 0o777); err != nil {
				t.Fatal(err)
			}
			return dir, "o/r"
		}},
		{"the clone's folder is a file", func(t *testing.T, f *cloneFixture, dir string) (string, string) {
			if err := os.Rename(dir, filepath.Join(f.home, "moved")); err != nil {
				t.Fatal(err)
			}
			writeFile(t, dir, "x")
			return dir, "o/r"
		}},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			made := f.madeClone(t)
			dir, origin := tc.make(t, f, made)
			before := snapshot(t, f.home)
			f.calls = nil
			s := CloneStatus(t.Context(), f.home, terminalproto.RepoCloneStatus{Dir: dir, Origin: origin}, f.git)
			if s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneDenied}) {
				t.Fatalf("state = %+v, want denied", s)
			}
			r := RemoveClone(t.Context(), f.home, terminalproto.RepoCloneRemove{Dir: dir, Origin: origin, Fingerprint: anyFingerprint}, f.git)
			wantKept(t, r, terminalproto.RepoCloneDenied, f.home, before)
			if len(f.calls) != 0 {
				t.Fatalf("git ran for a path that is not a clone's: %q", f.calls)
			}
		})
	}
}

// A count git could not give is never read as zero: the state says failed
// and a remove is refused, whatever the person was shown before.
func TestCloneStatusFailsWhenGitFailsForACount(t *testing.T) {
	for _, tc := range []struct {
		test string
		// fails reports the git call to fail.
		fails func(args []string) bool
		// make prepares the clone so that the call is made.
		make func(t *testing.T, dir string)
	}{
		{"the working tree's top", func(args []string) bool { return slices.Contains(args, "--show-toplevel") }, nil},
		{"the worktree list", func(args []string) bool { return gitSub(args) == "worktree" }, nil},
		{"status", func(args []string) bool { return gitSub(args) == "status" }, nil},
		{"the refs", func(args []string) bool { return gitSub(args) == "for-each-ref" }, nil},
		{"the unpushed count", func(args []string) bool {
			return gitSub(args) == "rev-list" && !slices.Contains(args, "--walk-reflogs")
		}, nil},
		{"the hidden entries", func(args []string) bool { return gitSub(args) == "ls-files" && slices.Contains(args, "-v") }, nil},
		{"the index entries", func(args []string) bool { return gitSub(args) == "ls-files" && slices.Contains(args, "--stage") }, nil},

		{"the stash count", func(args []string) bool { return slices.Contains(args, "--walk-reflogs") }, func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "README"), "stashed\n")
			testGit(t, dir, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "stash", "-q")
		}},
		{"a detached HEAD", func(args []string) bool { return slices.Contains(args, "HEAD^{commit}") }, func(t *testing.T, dir string) {
			testGit(t, dir, "checkout", "-q", "--detach")
		}},
		{"what is staged", func(args []string) bool { return gitSub(args) == "diff-index" }, func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "new"), "x")
		}},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			if tc.make != nil {
				tc.make(t, dir)
			}
			shown := f.status(t, dir, "o/r")
			if shown.Reason != "" {
				t.Fatalf("state before the failure = %+v", shown)
			}
			failed := 0
			failing := func(ctx context.Context, d string, args ...string) ([]byte, error) {
				if tc.fails(args) {
					failed++
					return nil, errors.New("exit status 1")
				}
				return f.git(ctx, d, args...)
			}
			before := snapshot(t, f.home)
			s := f.statusWith(t, dir, "o/r", failing)
			if s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneFailed}) || failed == 0 {
				t.Fatalf("state = %+v after %d failed calls, want failed", s, failed)
			}
			wantKept(t, f.removeWith(t, dir, "o/r", shown.Fingerprint, failing), terminalproto.RepoCloneFailed, f.home, before)
		})
	}
}

// Output git gave that is not what was asked for is a failure too.
func TestCloneStatusFailsOnUnreadableGitOutput(t *testing.T) {
	for _, tc := range []struct {
		test, sub, out string
	}{
		{"a count that is not a number", "rev-list", "many\n"},
		{"refs that are not refs", "for-each-ref", "nonsense\n"},
		{"a worktree list without the clone", "worktree", "worktree /somewhere/else\nHEAD 0123456789abcdef0123456789abcdef01234567\n\n"},
		{"an empty worktree list", "worktree", ""},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			odd := func(ctx context.Context, d string, args ...string) ([]byte, error) {
				if gitSub(args) == tc.sub {
					return []byte(tc.out), nil
				}
				return f.git(ctx, d, args...)
			}
			if s := f.statusWith(t, dir, "o/r", odd); s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneFailed}) {
				t.Fatalf("state = %+v, want failed", s)
			}
		})
	}
}

// What status cannot see, or what git keeps outside the checked refs, makes
// the clone not removable rather than clean.
func TestCloneStatusRefusesWhatItCannotVouchFor(t *testing.T) {
	for _, tc := range []struct {
		test   string
		reason terminalproto.RepoCloneReason
		make   func(t *testing.T, f *cloneFixture, dir string)
	}{
		{"a file git was told to assume unchanged", terminalproto.RepoCloneNotRemovable, func(t *testing.T, f *cloneFixture, dir string) {
			testGit(t, dir, "update-index", "--assume-unchanged", "README")
			writeFile(t, filepath.Join(dir, "README"), "edited where status does not look\n")
		}},
		{"a file git was told to skip", terminalproto.RepoCloneNotRemovable, func(t *testing.T, f *cloneFixture, dir string) {
			testGit(t, dir, "update-index", "--skip-worktree", "README")
			writeFile(t, filepath.Join(dir, "README"), "edited where status does not look\n")
		}},
		{"a submodule's repository", terminalproto.RepoCloneNotRemovable, func(t *testing.T, f *cloneFixture, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, ".git", "modules", "lib"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"a rebase in progress", terminalproto.RepoCloneNotRemovable, func(t *testing.T, f *cloneFixture, dir string) {
			// A rebase keeps what it set aside (its autostash) in this folder,
			// where neither status nor the refs show it.
			if err := os.Mkdir(filepath.Join(dir, ".git", "rebase-merge"), 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dir, ".git", "rebase-merge", "autostash"), "0123456789abcdef0123456789abcdef01234567\n")
		}},
		{"a patch series in progress", terminalproto.RepoCloneNotRemovable, func(t *testing.T, f *cloneFixture, dir string) {
			if err := os.Mkdir(filepath.Join(dir, ".git", "rebase-apply"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"a merge that set changes aside", terminalproto.RepoCloneNotRemovable, func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(dir, ".git", "MERGE_AUTOSTASH"), "0123456789abcdef0123456789abcdef01234567\n")
		}},
		{"a working tree set elsewhere", terminalproto.RepoCloneFailed, func(t *testing.T, f *cloneFixture, dir string) {
			elsewhere := filepath.Join(f.home, "elsewhere")
			if err := os.Mkdir(elsewhere, 0o700); err != nil {
				t.Fatal(err)
			}
			testGit(t, dir, "config", "core.worktree", elsewhere)
		}},
		{"a bare repository", terminalproto.RepoCloneFailed, func(t *testing.T, f *cloneFixture, dir string) {
			testGit(t, dir, "config", "core.bare", "true")
		}},
		{"a bisect in progress on unpushed commits", "", func(t *testing.T, f *cloneFixture, dir string) {
			commitIn(t, dir, "a")
			testGit(t, dir, "update-ref", "refs/bisect/bad", "HEAD")
			testGit(t, dir, "reset", "-q", "--hard", "origin/main")
		}},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			tc.make(t, f, dir)
			before := snapshot(t, f.home)
			s := f.status(t, dir, "o/r")
			if tc.reason == "" {
				wantCounts(t, s, 0, 0, 0, 1)
				wantKept(t, f.remove(t, dir, "o/r", s.Fingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
				return
			}
			if s != (terminalproto.RepoCloneState{Reason: tc.reason}) {
				t.Fatalf("state = %+v, want %s", s, tc.reason)
			}
			wantKept(t, f.remove(t, dir, "o/r", anyFingerprint), tc.reason, f.home, before)
		})
	}
}

// Review Focus 5: work that arrived after the person was shown the clone is
// never deleted.
func TestRemoveCloneRefusesWhenChangedSinceShown(t *testing.T) {
	for _, tc := range []struct {
		test   string
		change func(t *testing.T, f *cloneFixture, dir string)
	}{
		{"a file added", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(dir, "new"), "work\n")
		}},
		{"a commit made", func(t *testing.T, f *cloneFixture, dir string) { commitIn(t, dir, "a") }},
		{"a branch made", func(t *testing.T, f *cloneFixture, dir string) { testGit(t, dir, "branch", "task") }},
		{"a worktree added", func(t *testing.T, f *cloneFixture, dir string) {
			testGit(t, dir, "worktree", "add", "-q", "--detach", filepath.Join(f.home, "wt"))
		}},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			shown := f.status(t, dir, "o/r")
			if !shown.Removable() {
				t.Fatalf("shown = %+v", shown)
			}
			tc.change(t, f, dir)
			before := snapshot(t, f.home)
			wantKept(t, f.remove(t, dir, "o/r", shown.Fingerprint), terminalproto.RepoCloneChanged, f.home, before)
		})
	}
	// A fingerprint that was never shown removes nothing either.
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	before := snapshot(t, f.home)
	for _, fingerprint := range []string{anyFingerprint, "", "short"} {
		r := RemoveClone(t.Context(), f.home, terminalproto.RepoCloneRemove{Dir: dir, Origin: "o/r", Fingerprint: fingerprint}, f.git)
		wantKept(t, r, terminalproto.RepoCloneChanged, f.home, before)
	}
}

func TestRemoveCloneDeletesTheFolderAndAnEmptyOwner(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	// Ignored files are not counted and go with the clone.
	writeFile(t, filepath.Join(dir, ".git", "info", "exclude"), "ignored\n")
	writeFile(t, filepath.Join(dir, "ignored"), "x")
	shown := f.status(t, dir, "o/r")
	wantCounts(t, shown, 0, 0, 0, 0)
	outside := snapshot(t, f.remote)
	r := f.remove(t, dir, "o/r", shown.Fingerprint)
	if r != (terminalproto.RepoCloneRemoved{Dir: dir}) {
		t.Fatalf("remove = %+v", r)
	}
	wantMissing(t, dir)
	wantMissing(t, filepath.Dir(dir))
	if names := entries(t, f.repos()); len(names) != 0 {
		t.Fatalf("reet/repos holds %q", names)
	}
	if !slices.Equal(snapshot(t, f.remote), outside) {
		t.Fatal("the remove touched something outside the clone")
	}
	// A second remove finds nothing to remove.
	if again := f.remove(t, dir, "o/r", shown.Fingerprint); again != (terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneDenied}) {
		t.Fatalf("second remove = %+v", again)
	}
	if !realDir(f.repos()) {
		t.Fatal("reet/repos is gone")
	}
}

func TestRemoveCloneKeepsAnOwnerWithAnotherRepo(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	owner := filepath.Dir(dir)
	other := filepath.Join(owner, "other")
	f.handClone(t, other, "https://github.com/o/other.git")
	writeFile(t, filepath.Join(owner, "note"), "kept\n")
	kept := snapshot(t, other)
	shown := f.status(t, dir, "o/r")
	if r := f.remove(t, dir, "o/r", shown.Fingerprint); r != (terminalproto.RepoCloneRemoved{Dir: dir}) {
		t.Fatalf("remove = %+v", r)
	}
	if names := entries(t, owner); !slices.Equal(names, []string{"note", "other"}) {
		t.Fatalf("owner folder holds %q", names)
	}
	if !slices.Equal(snapshot(t, other), kept) {
		t.Fatal("the other repository changed")
	}
}

// A request whose time is already up looks at nothing and removes nothing.
func TestRemoveCloneRefusesWhenOutOfTime(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	shown := f.status(t, dir, "o/r")
	before := snapshot(t, f.home)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if s := CloneStatus(ctx, f.home, terminalproto.RepoCloneStatus{Dir: dir, Origin: "o/r"}, f.git); s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneTimedOut}) {
		t.Fatalf("state = %+v, want timed_out", s)
	}
	r := RemoveClone(ctx, f.home, terminalproto.RepoCloneRemove{Dir: dir, Origin: "o/r", Fingerprint: shown.Fingerprint}, f.git)
	wantKept(t, r, terminalproto.RepoCloneTimedOut, f.home, before)
	// And without git nothing is said or removed.
	if s := CloneStatus(t.Context(), f.home, terminalproto.RepoCloneStatus{Dir: dir, Origin: "o/r"}, nil); s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneFailed}) {
		t.Fatalf("state without git = %+v", s)
	}
	r = RemoveClone(t.Context(), f.home, terminalproto.RepoCloneRemove{Dir: dir, Origin: "o/r", Fingerprint: shown.Fingerprint}, nil)
	wantKept(t, r, terminalproto.RepoCloneFailed, f.home, before)
}

// A delete that fails is reported, and the owner folder is left alone.
func TestRemoveCloneReportsADeleteThatFails(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	shown := f.status(t, dir, "o/r")
	original := removeCloneDir
	var asked []string
	removeCloneDir = func(path string) error { asked = append(asked, path); return errors.New("read-only file system") }
	t.Cleanup(func() { removeCloneDir = original })
	before := snapshot(t, f.home)
	wantKept(t, f.remove(t, dir, "o/r", shown.Fingerprint), terminalproto.RepoCloneFailed, f.home, before)
	if !slices.Equal(asked, []string{dir}) {
		t.Fatalf("asked to delete %q, want only the clone", asked)
	}
}

// A clone that finds the folder already there still clears what earlier
// clones of the repository left behind.
func TestCloneRepoRemovesAStaleLeftoverWhenTheFolderIsThere(t *testing.T) {
	for _, tc := range []struct {
		test string
		make func(t *testing.T, f *cloneFixture, dest string)
		want terminalproto.RepoCloneResult
	}{
		{"adopted", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://github.com/o/r.git")
		}, terminalproto.RepoCloneResult{DefaultBranch: "main", Adopted: true}},
		{"refused", func(t *testing.T, f *cloneFixture, dest string) {
			f.handClone(t, dest, "https://github.com/o/other.git")
		}, terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneExists}},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			owner := filepath.Join(f.repos(), "o")
			dest := filepath.Join(owner, "r")
			tc.make(t, f, dest)
			makeLeftover(t, filepath.Join(owner, leftoverName("r")), 20*time.Minute)
			// One too young to be a leftover, and another repository's, stay.
			makeLeftover(t, filepath.Join(owner, ".r.cloning-fedcba9876543210"), time.Minute)
			makeLeftoverOf(t, filepath.Join(owner, leftoverName("other")), "https://github.com/o/other.git", time.Hour)
			kept := snapshot(t, dest)
			want := tc.want
			if want.Reason == "" {
				want.Dir = dest
			}
			if r := f.clone(t, "o", "r"); r != want {
				t.Fatalf("result = %+v, want %+v", r, want)
			}
			if names := entries(t, owner); !slices.Equal(names, []string{leftoverName("other"), ".r.cloning-fedcba9876543210", "r"}) {
				t.Fatalf("owner folder holds %q", names)
			}
			if !slices.Equal(snapshot(t, dest), kept) {
				t.Fatal("the folder that was there changed")
			}
			if calls := f.cloneCalls(); len(calls) != 0 {
				t.Fatalf("cloned: %q", calls)
			}
		})
	}
}

// fsmonitorHook configures, in the clone's own config, a core.fsmonitor hook
// that says nothing changed and leaves a marker each time it runs. It primes
// the index as a person's own `git status` would and returns the marker.
func fsmonitorHook(t *testing.T, f *cloneFixture, dir string) string {
	t.Helper()
	marker := filepath.Join(f.home, "hook-ran")
	hook := filepath.Join(f.home, "fsmonitor-hook")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho ran >> '"+marker+"'\nprintf 'token\\0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	testGit(t, dir, "config", "core.fsmonitor", hook)
	testGit(t, dir, "config", "core.fsmonitorHookVersion", "2")
	testGit(t, dir, "status")
	testGit(t, dir, "status")
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the hook did not run for the person's own status:", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	return marker
}

// A hook the clone's config names (core.fsmonitor) is never run, and cannot
// hide an edit from the count.
func TestCloneStatusRunsNoHookAndCountsWhatItWouldHide(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	marker := fsmonitorHook(t, f, dir)
	clean := f.status(t, dir, "o/r")
	wantCounts(t, clean, 0, 0, 0, 0)
	writeFile(t, filepath.Join(dir, "README"), "edited behind the hook\n")
	before := snapshot(t, f.home)
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 1, 0, 0)
	wantKept(t, f.remove(t, dir, "o/r", clean.Fingerprint), terminalproto.RepoCloneChanged, f.home, before)
	wantKept(t, f.remove(t, dir, "o/r", s.Fingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
	wantMissing(t, marker)
}

// Every git call a status or a remove makes carries the hardening.
func TestCloneStatusHardensEveryGitCall(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	writeFile(t, filepath.Join(dir, "new"), "x")
	testGit(t, dir, "checkout", "-q", "--detach")
	f.calls = nil
	s := f.status(t, dir, "o/r")
	wantCounts(t, s, 0, 1, 0, 0)
	f.remove(t, dir, "o/r", s.Fingerprint)
	if len(f.calls) < 16 {
		t.Fatalf("only %d git calls: %q", len(f.calls), f.calls)
	}
	for _, args := range f.calls {
		for _, want := range []string{"core.fsmonitor=false", "core.untrackedCache=false", "core.hooksPath=/dev/null", "diff.external=", "core.attributesFile=/dev/null",
			"filter.lfs.process=", "filter.lfs.smudge=", "filter.lfs.clean=", "filter.lfs.required=false"} {
			at := slices.Index(args, want)
			if at < 1 || args[at-1] != "-c" || at > slices.Index(args, gitSub(args)) {
				t.Fatalf("git call without -c %s before its subcommand: %q", want, args)
			}
		}
	}
}

// The read-only listings keep their large output bound behind -c arguments.
func TestOutputLimitLooksPastConfigArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"status", "--porcelain"}, gitListLimit},
		{[]string{"-c", "core.fsmonitor=false", "-c", "x=y", "status"}, gitListLimit},
		{[]string{"-c", "x=y", "for-each-ref"}, gitListLimit},
		{[]string{"-c", "x=y", "ls-files", "-v"}, gitListLimit},
		{[]string{"-c", "x=y", "worktree", "list"}, gitListLimit},
		{[]string{"-c", "x=y", "worktree", "remove"}, gitOutputLimit},
		{[]string{"-c", "x=y", "rev-parse"}, gitOutputLimit},
		{[]string{"-c", "status"}, gitOutputLimit},
		{[]string{"-c"}, gitOutputLimit},
		{nil, gitOutputLimit},
	} {
		if got := outputLimit(tc.args); got != tc.want {
			t.Errorf("outputLimit(%q) = %d, want %d", tc.args, got, tc.want)
		}
	}
}

// nestedRepo makes a repository with one commit that exists nowhere else.
func nestedRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	testGit(t, path, "init", "-q", "-b", "main")
	commitIn(t, path, "only-here")
}

// A repository inside the clone has history of its own that is on no
// remote: the clone is never removed with one in it, however it is hidden.
func TestCloneStatusRefusesANestedRepository(t *testing.T) {
	commit := func(t *testing.T, dir string) {
		t.Helper()
		testGit(t, dir, "-c", "user.name=Reet Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "sub")
	}
	for _, tc := range []struct {
		test string
		make func(t *testing.T, f *cloneFixture, dir string)
	}{
		{"committed as a gitlink and pushed", func(t *testing.T, f *cloneFixture, dir string) {
			nestedRepo(t, filepath.Join(dir, "sub"))
			testGit(t, dir, "-c", "advice.addEmbeddedRepo=false", "add", "sub")
			commit(t, dir)
			f.push(t, dir, "main")
		}},
		{"a gitlink whose folder is gone", func(t *testing.T, f *cloneFixture, dir string) {
			nestedRepo(t, filepath.Join(dir, "sub"))
			testGit(t, dir, "-c", "advice.addEmbeddedRepo=false", "add", "sub")
			commit(t, dir)
			f.push(t, dir, "main")
			if err := os.RemoveAll(filepath.Join(dir, "sub")); err != nil {
				t.Fatal(err)
			}
			// Told not to look for the folder, status says nothing of it.
			testGit(t, dir, "update-index", "--skip-worktree", "sub")
		}},
		{"inside an ignored folder", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(dir, ".git", "info", "exclude"), "vendor/\n")
			nestedRepo(t, filepath.Join(dir, "vendor", "deep", "lib"))
		}},
		{"a .git file inside an ignored folder", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(dir, ".git", "info", "exclude"), "vendor/\n")
			if err := os.MkdirAll(filepath.Join(dir, "vendor", "lib"), 0o700); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dir, "vendor", "lib", ".git"), "gitdir: /somewhere/else\n")
		}},
		{"a .git symlink inside an ignored folder", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(dir, ".git", "info", "exclude"), "vendor/\n")
			nestedRepo(t, filepath.Join(f.home, "outside"))
			if err := os.MkdirAll(filepath.Join(dir, "vendor", "lib"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(f.home, "outside", ".git"), filepath.Join(dir, "vendor", "lib", ".git")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			tc.make(t, f, dir)
			before := snapshot(t, f.home)
			s := f.status(t, dir, "o/r")
			if s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneNotRemovable}) {
				t.Fatalf("state = %+v, want not_removable", s)
			}
			wantKept(t, f.remove(t, dir, "o/r", anyFingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
		})
	}
}

// A folder the walk for nested repositories cannot read is a failure, not
// a clean clone. A symlink is not followed out of the clone.
func TestCloneStatusWalksTheCloneWithoutFollowingSymlinks(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	writeFile(t, filepath.Join(dir, ".git", "info", "exclude"), "vendor\n")
	nestedRepo(t, filepath.Join(f.home, "outside"))
	if err := os.Symlink(filepath.Join(f.home, "outside"), filepath.Join(dir, "vendor")); err != nil {
		t.Fatal(err)
	}
	wantCounts(t, f.status(t, dir, "o/r"), 0, 0, 0, 0)

	original := walkCloneDir
	walkCloneDir = func(root string, fn fs.WalkDirFunc) error {
		return original(root, func(path string, d fs.DirEntry, err error) error {
			if path == filepath.Join(dir, "README") {
				return fn(path, d, errors.New("input/output error"))
			}
			return fn(path, d, err)
		})
	}
	t.Cleanup(func() { walkCloneDir = original })
	before := snapshot(t, f.home)
	if s := f.status(t, dir, "o/r"); s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneFailed}) {
		t.Fatalf("state = %+v, want failed", s)
	}
	wantKept(t, f.remove(t, dir, "o/r", anyFingerprint), terminalproto.RepoCloneFailed, f.home, before)
}

// A lock file means git is writing in the clone right now.
func TestCloneStatusRefusesWhileGitIsWriting(t *testing.T) {
	for _, lock := range []string{"index.lock", "HEAD.lock", "packed-refs.lock", "shallow.lock", "config.lock",
		"refs/heads/main.lock", "refs/heads/reet/run.lock", "refs/remotes/origin/main.lock", "refs/stash.lock"} {
		t.Run(lock, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			path := filepath.Join(dir, ".git", filepath.FromSlash(lock))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			shown := f.status(t, dir, "o/r")
			writeFile(t, path, "")
			before := snapshot(t, f.home)
			if s := f.status(t, dir, "o/r"); s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneNotRemovable}) {
				t.Fatalf("state = %+v, want not_removable", s)
			}
			wantKept(t, f.remove(t, dir, "o/r", shown.Fingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
		})
	}
}

// When pushes do not go to this repository on GitHub, origin's
// remote-tracking branches say nothing about what GitHub has.
func TestCloneStatusRefusesPushesThatGoElsewhere(t *testing.T) {
	for _, tc := range []struct {
		test string
		make func(t *testing.T, f *cloneFixture, dir string)
		want terminalproto.RepoCloneReason
	}{
		{"a push address of its own", func(t *testing.T, f *cloneFixture, dir string) {
			testGit(t, dir, "config", "remote.origin.pushurl", filepath.Join(f.home, "backup.git"))
		}, terminalproto.RepoCloneExists},
		{"a push address naming the same repository", func(t *testing.T, f *cloneFixture, dir string) {
			testGit(t, dir, "config", "remote.origin.pushurl", "https://github.com/o/r.git")
		}, terminalproto.RepoCloneExists},
		{"an empty push address", func(t *testing.T, f *cloneFixture, dir string) {
			testGit(t, dir, "config", "remote.origin.pushurl", "")
		}, terminalproto.RepoCloneExists},
		{"pushes rewritten by the person's own config", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(f.home, ".gitconfig"), "[url \"https://example.com/\"]\n\tpushInsteadOf = https://github.com/\n")
		}, terminalproto.RepoCloneExists},
		{"every address rewritten by the person's own config", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(f.home, ".gitconfig"), "[url \"https://example.com/\"]\n\tinsteadOf = https://github.com/\n")
		}, terminalproto.RepoCloneExists},
		{"pushes rewritten to another repository", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(f.home, ".gitconfig"), "[url \"https://github.com/o/fork\"]\n\tpushInsteadOf = https://github.com/o/r\n")
		}, terminalproto.RepoCloneExists},
		{"github.com rewritten to ssh", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(f.home, ".gitconfig"), "[url \"git@github.com:\"]\n\tinsteadOf = https://github.com/\n")
		}, ""},
		{"pushes rewritten to ssh", func(t *testing.T, f *cloneFixture, dir string) {
			writeFile(t, filepath.Join(f.home, ".gitconfig"), "[url \"ssh://git@github.com/\"]\n\tpushInsteadOf = https://github.com/\n")
		}, ""},
	} {
		t.Run(tc.test, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			tc.make(t, f, dir)
			before := snapshot(t, f.home)
			s := f.status(t, dir, "o/r")
			if tc.want == "" {
				wantCounts(t, s, 0, 0, 0, 0)
				return
			}
			if s != (terminalproto.RepoCloneState{Reason: tc.want}) {
				t.Fatalf("state = %+v, want %s", s, tc.want)
			}
			wantKept(t, f.remove(t, dir, "o/r", anyFingerprint), tc.want, f.home, before)
		})
	}
	// A push address git could not give is not proof of one.
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	shown := f.status(t, dir, "o/r")
	failing := func(ctx context.Context, d string, args ...string) ([]byte, error) {
		if gitSub(args) == "remote" {
			return nil, errors.New("exit status 2")
		}
		return f.git(ctx, d, args...)
	}
	before := snapshot(t, f.home)
	if s := f.statusWith(t, dir, "o/r", failing); s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneExists}) {
		t.Fatalf("state = %+v, want exists", s)
	}
	wantKept(t, f.removeWith(t, dir, "o/r", shown.Fingerprint, failing), terminalproto.RepoCloneExists, f.home, before)
}

// A folder put in the clone's place after it was checked is not the folder
// that was checked, and is not deleted.
func TestRemoveCloneRefusesAFolderSwappedInAfterTheCheck(t *testing.T) {
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	shown := f.status(t, dir, "o/r")
	// A clone that passes every check, standing by to take the place.
	standby := filepath.Join(f.home, "standby")
	f.handClone(t, standby, "https://github.com/o/r.git")
	writeFile(t, filepath.Join(standby, ".git", cloneMarker), "o/r\n")
	writeFile(t, filepath.Join(standby, ".git", "info", "exclude"), "precious\n")
	writeFile(t, filepath.Join(standby, "precious"), "work\n")
	moved := filepath.Join(f.home, "moved")
	swaps := 0
	swapping := func(ctx context.Context, d string, args ...string) ([]byte, error) {
		out, err := f.git(ctx, d, args...)
		// The last thing git is asked: every check after it reads the disk.
		if gitSub(args) == "ls-files" && slices.Contains(args, "--stage") {
			swaps++
			if err := os.Rename(dir, moved); err != nil {
				t.Error(err)
			}
			if err := os.Rename(standby, dir); err != nil {
				t.Error(err)
			}
		}
		return out, err
	}
	var deleted []string
	original := removeCloneDir
	removeCloneDir = func(path string) error { deleted = append(deleted, path); return original(path) }
	t.Cleanup(func() { removeCloneDir = original })
	r := f.removeWith(t, dir, "o/r", shown.Fingerprint, swapping)
	if r != (terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneDenied}) || swaps != 1 {
		t.Fatalf("remove = %+v after %d swaps, want denied", r, swaps)
	}
	if len(deleted) != 0 {
		t.Fatalf("deleted %q", deleted)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "precious")); err != nil || string(data) != "work\n" {
		t.Fatalf("the folder swapped in was touched: %q %v", data, err)
	}
	if !realDir(filepath.Join(moved, ".git")) {
		t.Fatal("the folder that was checked is gone")
	}
}

// A folder another user owns anywhere on the way is not this user's clone.
// Only root can give a folder away, so this runs as root alone.
func TestRemoveCloneRefusesAFolderAnotherUserOwns(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("giving a folder to another user needs root")
	}
	for _, which := range []string{"reet", "reet/repos", "reet/repos/o", "reet/repos/o/r"} {
		t.Run(which, func(t *testing.T) {
			f := newCloneFixture(t)
			dir := f.madeClone(t)
			shown := f.status(t, dir, "o/r")
			if err := os.Lchown(filepath.Join(f.home, filepath.FromSlash(which)), 65534, -1); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, f.home)
			f.calls = nil
			if s := f.status(t, dir, "o/r"); s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneDenied}) {
				t.Fatalf("state = %+v, want denied", s)
			}
			wantKept(t, f.remove(t, dir, "o/r", shown.Fingerprint), terminalproto.RepoCloneDenied, f.home, before)
			if len(f.calls) != 0 {
				t.Fatalf("git ran in a folder another user owns: %q", f.calls)
			}
		})
	}
}
