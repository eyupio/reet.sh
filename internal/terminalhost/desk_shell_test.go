//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	deskPaneA = "pane0123456789abcdef"
	deskPaneB = "pane-fedcba9876543210"
)

// deskShellFixture is a manager whose plain shells are the named shell, with
// a temporary HOME holding the given start-up files. It skips when the shell
// is not installed. Nothing outside the temporary HOME is touched.
func deskShellFixture(t *testing.T, shell string, files map[string]string) (*Manager, string) {
	t.Helper()
	// REET_TEST_BASH / REET_TEST_ZSH run these tests against another build
	// of the shell (for example bash 3.2, which macOS ships).
	path, err := exec.LookPath(shell)
	if other := os.Getenv("REET_TEST_" + strings.ToUpper(shell)); other != "" {
		path, err = other, nil
	}
	if err != nil {
		t.Skipf("%s is not installed on this machine, so the %s wrapper was not run", shell, shell)
	}
	m, opts := managerFixture(t)
	m.shell = path
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(opts.Home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return m, opts.Home
}

func createDeskShell(t *testing.T, m *Manager, n int, pane string) ManagedSession {
	t.Helper()
	s, err := m.Create(context.Background(), testRequestID(n), testSessionID(n), terminalproto.Create{Title: "Shell", Cols: 120, Rows: 40, Desk: terminalproto.DeskRef{Pane: pane}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func screenOf(t *testing.T, m *Manager, id string) string {
	t.Helper()
	out, err := m.command(context.Background(), 256<<10, "capture-pane", "-p", "-J", "-S", "-200", "-t", "="+m.name(id)+":")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func waitScreen(t *testing.T, m *Manager, id, want string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		screen := screenOf(t, m, id)
		if strings.Contains(screen, want) {
			return screen
		}
		if time.Now().After(deadline) {
			t.Fatalf("screen never showed %q:\n%s", want, screen)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitFileHas(t *testing.T, path, want string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), want) {
			return string(data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never held %q; it holds:\n%s", path, want, data)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// killShell ends the session's shell with SIGKILL, as a crash or a power cut
// would: nothing it did not already write survives.
func killShell(t *testing.T, m *Manager, id string) {
	t.Helper()
	pid, err := strconv.Atoi(managedPID(t, m, id))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, err := m.command(context.Background(), 1024, "list-panes", "-t", "="+m.name(id), "-F", "#{pane_dead}")
		if err == nil && strings.TrimSpace(string(out)) == "1" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the killed shell's pane never died")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// keepsHistoryAcrossShells runs a command in a desk pane's shell, kills the
// shell, and checks a second shell for the same pane lists the command once.
func keepsHistoryAcrossShells(t *testing.T, shell string, files map[string]string) {
	m, home := deskShellFixture(t, shell, files)
	first := createDeskShell(t, m, 1, deskPaneA)
	marker := "echo reet-one-$((40+2))"
	typeInShell(t, m, first.ID, marker)
	waitScreen(t, m, first.ID, "reet-one-42")
	file := deskHistoryFile(home, m.shell, deskPaneA)
	waitFileHas(t, file, marker)
	killShell(t, m, first.ID)

	second := createDeskShell(t, m, 2, deskPaneA)
	typeInShell(t, m, second.ID, "history")
	screen := waitScreen(t, m, second.ID, marker)
	if n := strings.Count(screen, marker); n != 1 {
		t.Fatalf("the second shell lists the command %d times, want once:\n%s", n, screen)
	}
	// Another pane's shell has a history of its own.
	other := createDeskShell(t, m, 3, deskPaneB)
	typeInShell(t, m, other.ID, "history; echo reet-listed-$((1+1))")
	screen = waitScreen(t, m, other.ID, "reet-listed-2")
	if strings.Contains(screen, marker) {
		t.Fatalf("another pane's shell lists this pane's command:\n%s", screen)
	}
}

func TestBashWrapperKeepsHistoryAcrossShells(t *testing.T) {
	keepsHistoryAcrossShells(t, "bash", nil)
}

func TestZshWrapperKeepsHistoryAcrossShells(t *testing.T) {
	keepsHistoryAcrossShells(t, "zsh", nil)
}

// A login file that points HISTFILE elsewhere and sets its own PROMPT_COMMAND
// keeps its prompt command, but the pane's history still goes to the pane's
// file, applied after it.
func TestBashWrapperSurvivesRcThatOverridesHistfile(t *testing.T) {
	m, home := deskShellFixture(t, "bash", map[string]string{
		".bash_profile": "HISTFILE=$HOME/.elsewhere\nPROMPT_COMMAND='touch \"$HOME/prompt-ran\"'\n",
	})
	s := createDeskShell(t, m, 1, deskPaneA)
	typeInShell(t, m, s.ID, "echo reet-$((2+3))")
	waitScreen(t, m, s.ID, "reet-5")
	waitFileHas(t, deskHistoryFile(home, m.shell, deskPaneA), "echo reet-$((2+3))")
	if data, err := os.ReadFile(filepath.Join(home, ".elsewhere")); err == nil && strings.Contains(string(data), "reet-") {
		t.Fatalf("the command went to the file the profile chose: %s", data)
	}
	if _, err := os.Stat(filepath.Join(home, "prompt-ran")); err != nil {
		t.Fatalf("the person's own PROMPT_COMMAND did not run: %v", err)
	}
}

// The shell stays a login shell: a PATH set in ~/.profile is present.
func TestBashWrapperReadsProfilePath(t *testing.T) {
	m, _ := deskShellFixture(t, "bash", map[string]string{
		".profile": "PATH=\"$HOME/reet-profile-bin:$PATH\"\nexport PATH\n",
	})
	s := createDeskShell(t, m, 1, deskPaneA)
	typeInShell(t, m, s.ID, `case :$PATH: in *:$HOME/reet-profile-bin:*) echo reet-path-found-$((6*7));; *) echo reet-path-missing-$((6*7));; esac`)
	deadline := time.Now().Add(10 * time.Second)
	for {
		screen := screenOf(t, m, s.ID)
		if strings.Contains(screen, "reet-path-found-42") {
			return
		}
		if strings.Contains(screen, "reet-path-missing-42") || time.Now().After(deadline) {
			t.Fatalf("the PATH from ~/.profile is not in the shell:\n%s", screen)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// A start-up file that fails (a failing command, a syntax error, a return
// with an error) does not stop the shell or its history.
func TestWrapperSurvivesFailingRc(t *testing.T) {
	m, home := deskShellFixture(t, "bash", map[string]string{
		".bash_profile": "false\nif then fi\nreturn 3\necho never\n",
	})
	s := createDeskShell(t, m, 1, deskPaneA)
	typeInShell(t, m, s.ID, "echo reet-alive-$((1+1))")
	waitScreen(t, m, s.ID, "reet-alive-2")
	waitFileHas(t, deskHistoryFile(home, m.shell, deskPaneA), "echo reet-alive-$((1+1))")
}

// zsh: the person's own .zshenv, .zprofile, .zshrc and .zlogin are read from
// their home, and the pane's history still goes to the pane's file.
func TestZshWrapperSourcesUserRc(t *testing.T) {
	m, home := deskShellFixture(t, "zsh", map[string]string{
		".zshenv":   "export REET_SAW_ZSHENV=1\n",
		".zprofile": "export REET_SAW_ZPROFILE=1\n",
		".zshrc":    "REET_SAW_ZSHRC=1\nHISTFILE=$HOME/.elsewhere\n",
		".zlogin":   "REET_SAW_ZLOGIN=1\n",
	})
	s := createDeskShell(t, m, 1, deskPaneA)
	typeInShell(t, m, s.ID, "echo reet-saw-$REET_SAW_ZSHENV$REET_SAW_ZPROFILE$REET_SAW_ZSHRC$REET_SAW_ZLOGIN-${ZDOTDIR:-none}")
	waitScreen(t, m, s.ID, "reet-saw-1111-none")
	waitFileHas(t, deskHistoryFile(home, m.shell, deskPaneA), "reet-saw-")
}

// A shell that is neither bash nor zsh starts exactly as a plain create, with
// no wrapper written, and the history read says the pane has none.
func TestUnknownShellLeftAlone(t *testing.T) {
	if argv, env, ok := deskShellCommand("/home/me", "/bin/sh", deskPaneA, true); ok || argv != nil || env != nil {
		t.Fatalf("sh got a wrapper: %v %v %v", argv, env, ok)
	}
	if argv, env, ok := deskShellCommand("/home/me", "/usr/bin/fish", deskPaneA, true); ok || argv != nil || env != nil {
		t.Fatalf("fish got a wrapper: %v %v %v", argv, env, ok)
	}
	m, opts := managerFixture(t)
	m.shell = "/bin/sh"
	s := createDeskShell(t, m, 1, deskPaneA)
	if s.State != terminalproto.Running {
		t.Fatalf("state %s", s.State)
	}
	if _, err := os.Stat(filepath.Join(opts.Home, ".reet", "desks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a wrapper was written for sh: %v", err)
	}
	history := readHistory(t, m, s.ID, true)
	if len(history.Sources) == 0 || history.Sources[0] != (terminalproto.CommandHistorySource{Source: "pane", Reason: deskNoHistoryReason}) {
		t.Fatalf("sources %+v", history.Sources)
	}
}

// A wrapper that cannot be written never stops the shell: it starts as a
// plain create and the pane reports no history of its own.
func TestDeskShellStartsPlainWhenWrapperCannotBeWritten(t *testing.T) {
	m, home := deskShellFixture(t, "bash", nil)
	if err := os.WriteFile(filepath.Join(home, ".reet"), []byte("not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := createDeskShell(t, m, 1, deskPaneA)
	if s.State != terminalproto.Running {
		t.Fatalf("state %s", s.State)
	}
	history := readHistory(t, m, s.ID, true)
	if len(history.Sources) == 0 || history.Sources[0] != (terminalproto.CommandHistorySource{Source: "pane", Reason: deskSetupFailedReason}) {
		t.Fatalf("sources %+v", history.Sources)
	}
}

// A pane id is checked before it is used in a path: one that would climb out
// of the history folder is refused everywhere.
func TestDeskPaneIDRejectedInPath(t *testing.T) {
	home := t.TempDir()
	for _, pane := range []string{"../x", "../../../../etc/passwd", "../0123456789abcdef", "a/0123456789abcdef", "", ".0123456789abcdef"} {
		if file := deskHistoryFile(home, "/bin/bash", pane); file != "" {
			t.Fatalf("%q gave history file %q", pane, file)
		}
		if _, _, ok := deskShellCommand(home, "/bin/bash", pane, true); ok {
			t.Fatalf("%q got a wrapper", pane)
		}
		if err := prepareDeskShell(home, "/bin/bash", pane); err == nil {
			t.Fatalf("%q was prepared", pane)
		}
		m := &Manager{home: home}
		if err := m.forgetDeskPane(pane); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%q forget: %v", pane, err)
		}
		create := terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24, Desk: terminalproto.DeskRef{Pane: pane}}
		if pane != "" && validCreate(create) {
			t.Fatalf("%q accepted in a create", pane)
		}
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("refused panes left files: %v", entries)
	}
	want := filepath.Join(home, ".reet", "desks", "history", deskPaneA+".bash")
	if got := deskHistoryFile(home, "/bin/bash", deskPaneA); got != want {
		t.Fatalf("history file %q, want %q", got, want)
	}
}

func validCreate(c terminalproto.Create) bool {
	data, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: testRequestID(1), SessionID: testSessionID(1), Body: c})
	if err != nil {
		return false
	}
	_, err = terminalproto.Decode(data)
	return err == nil
}

// Files under ~/.reet/desks are 0600 in 0700 folders, also when the folders
// were there before with wider modes, and after the shell has written.
func TestHistoryFileMode(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".reet", "desks", "history"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(home, ".reet", "desks"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, shell := range []string{"/bin/bash", "/bin/zsh"} {
		if err := prepareDeskShell(home, shell, deskPaneA); err != nil {
			t.Fatal(err)
		}
	}
	checkDeskModes(t, home)

	m, home := deskShellFixture(t, "bash", nil)
	s := createDeskShell(t, m, 1, deskPaneA)
	typeInShell(t, m, s.ID, "echo reet-mode")
	waitFileHas(t, deskHistoryFile(home, m.shell, deskPaneA), "echo reet-mode")
	checkDeskModes(t, home)
}

func checkDeskModes(t *testing.T, home string) {
	t.Helper()
	root := filepath.Join(home, ".reet", "desks")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s is %v, want %v", path, info.Mode().Perm(), want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func readHistory(t *testing.T, m *Manager, id string, deskShell bool) terminalproto.CommandHistorySnapshot {
	t.Helper()
	data, err := m.CommandHistoryFor(context.Background(), id, deskShell)
	if err != nil {
		t.Fatal(err)
	}
	history, err := terminalproto.DecodeCommandHistory(data)
	if err != nil {
		t.Fatal(err)
	}
	return history
}

// A desk pane's own history comes first, marked "pane", then the machine's
// shared files; a connection without desk-shell-v1 sees only the shared ones.
func TestCommandHistoryPaneFirst(t *testing.T) {
	m, home := deskShellFixture(t, "bash", nil)
	s := createDeskShell(t, m, 1, deskPaneA)
	// The shared file is newer than the pane's: the pane still comes first.
	if err := os.WriteFile(filepath.Join(home, ".bash_history"), []byte("#2000000000\nshared newer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deskHistoryFile(home, m.shell, deskPaneA), []byte("#1500000000\npane older\n#1500000001\npane newest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	history := readHistory(t, m, s.ID, true)
	var got []string
	for _, entry := range history.Entries {
		got = append(got, entry.Source+":"+entry.Command)
	}
	if strings.Join(got, "|") != "pane:pane newest|pane:pane older|bash:shared newer" {
		t.Fatalf("entries %v", got)
	}
	if len(history.Sources) != 3 || history.Sources[0] != (terminalproto.CommandHistorySource{Source: "pane", Available: true}) {
		t.Fatalf("sources %+v", history.Sources)
	}
	old := readHistory(t, m, s.ID, false)
	for _, source := range old.Sources {
		if source.Source == "pane" {
			t.Fatal("a pane source reached a connection without desk-shell-v1")
		}
	}
	if len(old.Entries) != 1 || old.Entries[0].Command != "shared newer" {
		t.Fatalf("old entries %+v", old.Entries)
	}
	// A shell that is not a desk pane's has no pane source.
	plain := createManaged(t, m, 2)
	for _, source := range readHistory(t, m, plain.ID, true).Sources {
		if source.Source == "pane" {
			t.Fatal("a plain shell has a pane source")
		}
	}
}

// Ending a desk pane's shell for good removes that pane's history file and no
// other, now and once more after the shell has had time to exit.
func TestForgetDeskPaneRemovesFile(t *testing.T) {
	old := deskForgetAgain
	deskForgetAgain = 50 * time.Millisecond
	t.Cleanup(func() { deskForgetAgain = old })
	m, home := deskShellFixture(t, "bash", nil)
	s := createDeskShell(t, m, 1, deskPaneA)
	other := createDeskShell(t, m, 2, deskPaneB)
	typeInShell(t, m, s.ID, "echo reet-forget")
	file := deskHistoryFile(home, m.shell, deskPaneA)
	waitFileHas(t, file, "echo reet-forget")
	if _, err := m.End(context.Background(), testRequestID(9), s.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.ForgetDeskSession(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("history file still there: %v", err)
	}
	// Written again by a shell on its way out: removed on the second pass.
	if err := os.WriteFile(file, []byte("late\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitNoPendingForget(t, m)
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("history file written late is still there: %v", err)
	}
	if _, err := os.Stat(deskHistoryFile(home, m.shell, deskPaneB)); err != nil {
		t.Fatalf("another pane's history went too: %v", err)
	}
	// A session that is not a desk pane's has nothing to forget.
	plain := createManaged(t, m, 3)
	if err := m.ForgetDeskSession(context.Background(), plain.ID); err != nil {
		t.Fatal(err)
	}
	_ = other
}

// waitNoPendingForget waits until no second removal is waiting, so none fires
// after the test's temporary HOME is gone.
func waitNoPendingForget(t *testing.T, m *Manager) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for m.pendingDeskForgets() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("a second history removal is still waiting")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A shell started for the same pane before the second removal calls it off:
// the new shell's history is kept.
func TestForgetDeskPaneKeepsNewShellsHistory(t *testing.T) {
	old := deskForgetAgain
	deskForgetAgain = 200 * time.Millisecond
	t.Cleanup(func() { deskForgetAgain = old })
	m, home := deskShellFixture(t, "bash", nil)
	s := createDeskShell(t, m, 1, deskPaneA)
	if _, err := m.End(context.Background(), testRequestID(9), s.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.ForgetDeskSession(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if m.pendingDeskForgets() != 1 {
		t.Fatal("no second removal was scheduled")
	}
	again := createDeskShell(t, m, 2, deskPaneA)
	if m.pendingDeskForgets() != 0 {
		t.Fatal("the new shell did not call off the second removal")
	}
	typeInShell(t, m, again.ID, "echo reet-kept")
	file := deskHistoryFile(home, m.shell, deskPaneA)
	waitFileHas(t, file, "echo reet-kept")
	time.Sleep(3 * deskForgetAgain)
	if data, err := os.ReadFile(file); err != nil || !strings.Contains(string(data), "echo reet-kept") {
		t.Fatalf("the new shell's history went: %q %v", data, err)
	}
}

// Close calls off a waiting second removal.
func TestCloseStopsPendingForget(t *testing.T) {
	m, _ := deskShellFixture(t, "bash", nil)
	s := createDeskShell(t, m, 1, deskPaneA)
	if err := m.ForgetDeskSession(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if m.pendingDeskForgets() != 1 {
		t.Fatal("no second removal was scheduled")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if m.pendingDeskForgets() != 0 {
		t.Fatal("Close left a second removal waiting")
	}
}

// A SHELL naming a bash that is not there starts the plain shell exactly as
// today (tmux falls back to /bin/sh), not the missing path, and the pane
// reports no history of its own.
func TestDeskShellMissingShellStartsPlain(t *testing.T) {
	m, opts := managerFixture(t)
	m.shell = filepath.Join(opts.Home, "no-such", "bash")
	s := createDeskShell(t, m, 1, deskPaneA)
	if s.State != terminalproto.Running {
		t.Fatalf("state %s", s.State)
	}
	typeInShell(t, m, s.ID, "echo reet-alive-$((3+4))")
	waitScreen(t, m, s.ID, "reet-alive-7")
	if _, err := os.Stat(filepath.Join(opts.Home, ".reet", "desks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a wrapper was written for a missing shell: %v", err)
	}
	history := readHistory(t, m, s.ID, true)
	if len(history.Sources) == 0 || history.Sources[0] != (terminalproto.CommandHistorySource{Source: "pane", Reason: deskNoHistoryReason}) {
		t.Fatalf("sources %+v", history.Sources)
	}
}

// The history read uses the shell kind recorded at create, not SHELL as it is
// later.
func TestCommandHistoryUsesShellRecordedAtCreate(t *testing.T) {
	m, home := deskShellFixture(t, "bash", nil)
	s := createDeskShell(t, m, 1, deskPaneA)
	if err := os.WriteFile(deskHistoryFile(home, "bash", deskPaneA), []byte("make test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.shell = "/bin/sh"
	history := readHistory(t, m, s.ID, true)
	if len(history.Entries) == 0 || history.Entries[0] != (terminalproto.CommandHistoryEntry{Command: "make test", Source: "pane", Timestamp: history.Entries[0].Timestamp}) {
		t.Fatalf("entries %+v sources %+v", history.Entries, history.Sources)
	}
}

// A ~/.bash_profile that sources ~/.bashrc, as most do, loads it once.
func TestBashWrapperLoadsBashrcOnce(t *testing.T) {
	m, home := deskShellFixture(t, "bash", map[string]string{
		".bash_profile": ". \"$HOME/.bashrc\"\n",
		".bashrc":       "echo loaded >> \"$HOME/bashrc-loads\"\n",
	})
	s := createDeskShell(t, m, 1, deskPaneA)
	typeInShell(t, m, s.ID, "echo reet-ready-$((1+1))")
	waitScreen(t, m, s.ID, "reet-ready-2")
	data, err := os.ReadFile(filepath.Join(home, "bashrc-loads"))
	if err != nil || strings.Count(string(data), "loaded") != 1 {
		t.Fatalf("~/.bashrc loaded %d times: %q %v", strings.Count(string(data), "loaded"), data, err)
	}
}

// The first command a shell with no pane history yet runs is on disk at the
// next prompt, and the shell's own list holds only the person's commands.
// (bash 3.2 saves nothing with history -a until its list held a line from
// before the shell; the wrapper's placeholder covers that and is dropped.)
func TestBashWrapperSavesFirstCommandAndHidesPlaceholder(t *testing.T) {
	m, home := deskShellFixture(t, "bash", nil)
	s := createDeskShell(t, m, 1, deskPaneA)
	file := deskHistoryFile(home, m.shell, deskPaneA)
	typeInShell(t, m, s.ID, "echo reet-a-$((1))")
	waitFileHas(t, file, "echo reet-a-")
	typeInShell(t, m, s.ID, "echo reet-b-$((1))")
	waitFileHas(t, file, "echo reet-b-")
	typeInShell(t, m, s.ID, "history; echo reet-listed-$((2))")
	screen := waitScreen(t, m, s.ID, "reet-listed-2")
	data, _ := os.ReadFile(file)
	if strings.Contains(screen, deskBashSeedLine) || strings.Contains(string(data), deskBashSeedLine) {
		t.Fatalf("the placeholder shows:\n%s\nfile:\n%s", screen, data)
	}
}

// bashVersion is the major and minor version of the bash at path.
func bashVersion(t *testing.T, path string) (int, int) {
	t.Helper()
	out, err := exec.Command(path, "-c", `echo "${BASH_VERSINFO[0]} ${BASH_VERSINFO[1]}"`).Output()
	if err != nil {
		t.Fatal(err)
	}
	var major, minor int
	if _, err := fmt.Sscan(string(out), &major, &minor); err != nil {
		t.Fatal(err)
	}
	return major, minor
}

// PROMPT_COMMAND as an array (bash 5.1 and later) keeps its other elements.
func TestBashWrapperKeepsPromptCommandArray(t *testing.T) {
	m, home := deskShellFixture(t, "bash", map[string]string{
		".bash_profile": "PROMPT_COMMAND=('touch \"$HOME/pc0\"' 'touch \"$HOME/pc1\"')\n",
	})
	if major, minor := bashVersion(t, m.shell); major < 5 || major == 5 && minor < 1 {
		t.Skipf("bash %d.%d does not run PROMPT_COMMAND as an array", major, minor)
	}
	s := createDeskShell(t, m, 1, deskPaneA)
	typeInShell(t, m, s.ID, "echo reet-array-$((1+1))")
	waitScreen(t, m, s.ID, "reet-array-2")
	waitFileHas(t, deskHistoryFile(home, m.shell, deskPaneA), "echo reet-array-")
	for _, name := range []string{"pc0", "pc1"} {
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(filepath.Join(home, name)); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("PROMPT_COMMAND element %s did not run", name)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
