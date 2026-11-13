//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
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
		}, false},
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
	if len(asked) != 1 || !slices.Equal(asked[0], []string{stale, "config", "--get", "remote.origin.url"}) {
		t.Fatalf("asked %q", asked)
	}
}
