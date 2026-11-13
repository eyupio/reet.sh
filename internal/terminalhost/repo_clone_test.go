//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"os"
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
	return &cloneFixture{home: home, remote: remote}
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
		rewrite := []string{"-c", "url." + f.remote + ".insteadOf=https://github.com/o/r.git"}
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
	for _, banned := range []string{"--depth", "--filter", "--bare", "--mirror", "--shallow-since", "--single-branch"} {
		for _, arg := range args {
			if strings.HasPrefix(arg, banned) {
				t.Fatalf("clone argv has %s: %q", banned, args)
			}
		}
	}
	want := append(networkGitArgs(), "clone", "--no-tags", "https://github.com/o/r.git")
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
