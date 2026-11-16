//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
	"golang.org/x/sys/unix"
)

// A plain shell opened for a desk pane (Create.Desk.Pane, desk-shell-v1) is
// started through a small wrapper under ~/.reet/desks/shell/ that loads the
// person's own bash or zsh start-up files as a login shell would, then gives
// the pane its own history file under ~/.reet/desks/history/, appended as
// each command runs, so a later shell for the same pane loads it. History
// stays on the machine. Any other shell starts exactly as a plain create.

// deskForgetAgain is how long after an end the pane's history file is
// removed a second time: a shell that was running a command when tmux hung
// it up saves that command on its way out, after the first removal.
var deskForgetAgain = 3 * time.Second

// deskNoHistoryReason and deskSetupFailedReason are what the history read
// says for a desk pane whose shell keeps no history of its own.
const (
	deskNoHistoryReason   = "This pane's own history needs bash or zsh."
	deskSetupFailedReason = "This pane's own history could not be set up on this machine."
)

func deskRoot(home string) string       { return filepath.Join(home, ".reet", "desks") }
func deskShellDir(home string) string   { return filepath.Join(deskRoot(home), "shell") }
func deskHistoryDir(home string) string { return filepath.Join(deskRoot(home), "history") }
func deskBashRC(home string) string     { return filepath.Join(deskShellDir(home), "bashrc") }
func deskZshDotDir(home string) string  { return filepath.Join(deskShellDir(home), "zsh") }
func deskShellKind(shell string) string {
	switch filepath.Base(shell) {
	case "bash":
		return "bash"
	case "zsh":
		return "zsh"
	}
	return ""
}

// deskHistoryFile is the pane's own history file for shell, or "" for a pane
// id that is not a terminalproto.ValidID or a shell that is neither bash nor
// zsh. The id is checked before it is used in a path.
func deskHistoryFile(home, shell, pane string) string {
	kind := deskShellKind(shell)
	if kind == "" || !terminalproto.ValidID(pane) {
		return ""
	}
	return filepath.Join(deskHistoryDir(home), pane+"."+kind)
}

// deskShellCommand is the command and the extra environment that start shell
// through the desk wrapper for pane: for bash its rc file, for zsh its
// ZDOTDIR. login keeps the login shell a plain create starts today. ok is
// false for a shell that is neither bash nor zsh or a pane id that is not a
// ValidID: that shell starts exactly as a plain create.
func deskShellCommand(home, shell, pane string, login bool) (argv []string, env []string, ok bool) {
	if deskHistoryFile(home, shell, pane) == "" || !filepath.IsAbs(shell) {
		return nil, nil, false
	}
	env = []string{"REET_DESK_PANE=" + pane}
	switch deskShellKind(shell) {
	case "bash":
		// bash reads --rcfile only when it is not a login shell, so the rc
		// file loads the login files itself when REET_DESK_LOGIN is set.
		if login {
			env = append(env, "REET_DESK_LOGIN=1")
		}
		return []string{shell, "--rcfile", deskBashRC(home), "-i"}, env, true
	default:
		env = append(env, "ZDOTDIR="+deskZshDotDir(home))
		if login {
			return []string{shell, "-l"}, env, true
		}
		return []string{shell, "-i"}, env, true
	}
}

// shellQuoteLiteral quotes s for a POSIX shell (bash and zsh alike).
func shellQuoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// deskBashRCText loads the person's own files the way bash would for a login
// (or, without REET_DESK_LOGIN, an interactive) shell, each guarded so a file
// that fails does not stop the shell, then applies the pane's history last so
// those files cannot undo it. bash itself reads HISTFILE after this file, so
// the history is not read here again (that would load every line twice).
func deskBashRCText(home string) string {
	return `# Written by reet for a desk pane's shell; rewritten each time one starts.
if [ "${REET_DESK_LOGIN-}" = 1 ]; then
	unset REET_DESK_LOGIN
	if [ -r /etc/profile ]; then . /etc/profile || :; fi
	for __reet_rc in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
		if [ -r "$__reet_rc" ]; then . "$__reet_rc" || :; break; fi
	done
	unset __reet_rc
else
	if [ -r "$HOME/.bashrc" ]; then . "$HOME/.bashrc" || :; fi
fi
case ${REET_DESK_PANE-} in
'' | *[!A-Za-z0-9_-]*) ;;
*)
	HISTFILE=` + shellQuoteLiteral(deskHistoryDir(home)) + `/"$REET_DESK_PANE".bash
	shopt -s histappend
	if [ "${BASH_VERSINFO[0]}" -lt 4 ] && [ ! -s "$HISTFILE" ]; then
		# bash 3.2 (macOS) appends with history -a only while its list holds
		# a line from before this shell, so a pane with no history yet would
		# save nothing until the shell exits. A placeholder is read into the
		# list (never into the file) and dropped once the first command is
		# saved.
		history -r ` + shellQuoteLiteral(deskBashSeed(home)) + `
		__reet_unseed() {
			# Until a command follows the placeholder, keep it.
			case $(fc -ln -1 2>/dev/null) in *'` + deskBashSeedLine + `'*) return ;; esac
			case $(fc -ln 1 1 2>/dev/null) in *'` + deskBashSeedLine + `'*) history -d 1 ;; esac
			__reet_unseed() { :; }
		}
		PROMPT_COMMAND="history -a; __reet_unseed${PROMPT_COMMAND:+
$PROMPT_COMMAND}"
	else
		PROMPT_COMMAND="history -a${PROMPT_COMMAND:+
$PROMPT_COMMAND}"
	fi
	;;
esac
`
}

// deskBashSeedLine is the placeholder bash 3.2 is given for a pane with no
// history yet (see deskBashRCText). Run by mistake, it does nothing.
const deskBashSeedLine = ": reet desk history"

func deskBashSeed(home string) string { return filepath.Join(deskShellDir(home), "bash-seed") }

// deskZshSource sources the person's own copy of name from their ZDOTDIR (or
// home) at the top level, so their typeset and setopt lines keep their usual
// scope, and notices a ZDOTDIR their file sets for the files after it.
func deskZshSource(name string) string {
	return `__reet_zdotdir=$ZDOTDIR
ZDOTDIR=${REET_USER_ZDOTDIR:-$HOME}
if [[ -r $ZDOTDIR/` + name + ` ]]; then source "$ZDOTDIR/` + name + `" || :; fi
REET_USER_ZDOTDIR=$ZDOTDIR
ZDOTDIR=$__reet_zdotdir
unset __reet_zdotdir
`
}

// deskZshRestore hands ZDOTDIR back to the person's own once start-up is done,
// so a zsh started from this one reads their files, not the wrapper.
const deskZshRestore = `if [[ -n ${REET_USER_ZDOTDIR-} && $REET_USER_ZDOTDIR != $HOME ]]; then
	export ZDOTDIR=$REET_USER_ZDOTDIR
else
	unset ZDOTDIR
fi
unset REET_USER_ZDOTDIR
`

func deskZshFiles(home string) map[string]string {
	const header = "# Written by reet for a desk pane's shell; rewritten each time one starts.\n"
	historyFile := shellQuoteLiteral(deskHistoryDir(home)) + `/$REET_DESK_PANE.zsh`
	// zsh reads HISTFILE itself once its start-up files have run. A zsh with
	// no SAVEHIST saves nothing, so one is given (at least HISTSIZE).
	history := `case ${REET_DESK_PANE-} in
'' | *[^A-Za-z0-9_-]*) ;;
*)
	HISTFILE=` + historyFile + `
	(( SAVEHIST > 0 )) || SAVEHIST=$(( HISTSIZE > 1000 ? HISTSIZE : 1000 ))
	(( HISTSIZE >= SAVEHIST )) || HISTSIZE=$SAVEHIST
	setopt INC_APPEND_HISTORY
	;;
esac
`
	// A .zlogin of the person's that points HISTFILE elsewhere is undone.
	again := `case ${REET_DESK_PANE-} in
'' | *[^A-Za-z0-9_-]*) ;;
*) HISTFILE=` + historyFile + ` ;;
esac
`
	return map[string]string{
		".zshenv":   header + deskZshSource(".zshenv"),
		".zprofile": header + deskZshSource(".zprofile"),
		".zshrc":    header + deskZshSource(".zshrc") + history + "if [[ ! -o login ]]; then\n" + deskZshRestore + "fi\n",
		".zlogin":   header + deskZshSource(".zlogin") + again + deskZshRestore,
	}
}

// privateDir makes dir (0700) when it is missing, and otherwise checks it is
// a real folder of the machine's user, made private if it was not.
func privateDir(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: %s is not a folder", ErrUnsafePath, dir)
	}
	if err := ownedByCurrentUser(info); err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// privateHistoryFile makes the pane's history file (0600) when it is missing,
// so the shell appends to a private file, and refuses one that is not a plain
// file of the machine's user.
func privateHistoryFile(path string) error {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), "desk-history")
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, path)
	}
	if err := ownedByCurrentUser(info); err != nil {
		return err
	}
	if info.Mode().Perm() != 0o600 {
		return file.Chmod(0o600)
	}
	return nil
}

// prepareDeskShell writes the wrapper for shell (atomically, 0600 in 0700
// folders) and the pane's history file. Any error means the shell is started
// as a plain create instead.
func prepareDeskShell(home, shell, pane string) error {
	historyFile := deskHistoryFile(home, shell, pane)
	if historyFile == "" {
		return terminalproto.ErrInvalid
	}
	dirs := []string{filepath.Join(home, ".reet"), deskRoot(home), deskShellDir(home), deskHistoryDir(home)}
	kind := deskShellKind(shell)
	if kind == "zsh" {
		dirs = append(dirs, deskZshDotDir(home))
	}
	for _, dir := range dirs {
		if dir == filepath.Join(home, ".reet") {
			// reet's own folder may already be shared by other features; it
			// is made if missing and only checked to be a real folder.
			if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			if info, err := os.Lstat(dir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: %s is not a folder", ErrUnsafePath, dir)
			}
			continue
		}
		if err := privateDir(dir); err != nil {
			return err
		}
	}
	if kind == "bash" {
		if err := writePrivateFile(deskBashSeed(home), []byte(deskBashSeedLine+"\n")); err != nil {
			return err
		}
		if err := writePrivateFile(deskBashRC(home), []byte(deskBashRCText(home))); err != nil {
			return err
		}
	} else {
		for name, text := range deskZshFiles(home) {
			if err := writePrivateFile(filepath.Join(deskZshDotDir(home), name), []byte(text)); err != nil {
				return err
			}
		}
	}
	return privateHistoryFile(historyFile)
}

// loginShell is the shell a plain create starts: tmux's default shell, which
// is the validated SHELL in the environment the manager gives tmux.
func (m *Manager) loginShell() string {
	if m.shell != "" {
		return m.shell
	}
	for _, entry := range m.environment() {
		if value, ok := strings.CutPrefix(entry, "SHELL="); ok {
			return value
		}
	}
	return ""
}

// deskStart is how a desk pane's shell was started: the shell kind whose
// history file it keeps, or why it keeps none. The history read uses it rather
// than looking at SHELL again.
type deskStart struct {
	kind, reason string
}

// executableFile reports whether path is a regular file someone may run, the
// check tmux makes before it uses a default shell (falling back to /bin/sh).
func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// deskShellStart is the command and environment for a desk pane's shell, or
// nothing (a plain create, exactly as today) for a shell without history
// support, a shell that is not there to run, or when the wrapper could not be
// written. A removal of the pane's history still pending from an earlier end
// is called off first. The caller holds m.mu.
func (m *Manager) deskShellStart(sessionID, pane string) (argv, env []string) {
	m.cancelDeskForget(pane)
	shell := m.loginShell()
	argv, env, ok := deskShellCommand(m.home, shell, pane, true)
	switch {
	case !ok || !executableFile(shell):
		m.deskStarts[sessionID] = deskStart{reason: deskNoHistoryReason}
		return nil, nil
	case prepareDeskShell(m.home, shell, pane) != nil:
		m.deskStarts[sessionID] = deskStart{reason: deskSetupFailedReason}
		return nil, nil
	}
	m.deskStarts[sessionID] = deskStart{kind: deskShellKind(shell)}
	return argv, env
}

// deskStartOf is how the session's desk shell was started: as recorded at
// create, or, for a session this manager did not start (reet restarted since),
// worked out from the shell a plain create starts. The caller holds m.mu.
func (m *Manager) deskStartOf(sessionID string) deskStart {
	if start, ok := m.deskStarts[sessionID]; ok {
		return start
	}
	shell := m.loginShell()
	if deskShellKind(shell) == "" || !executableFile(shell) {
		return deskStart{reason: deskNoHistoryReason}
	}
	return deskStart{kind: deskShellKind(shell)}
}

// deskPaneOf is the desk pane the session was created for, from its create
// receipt, or "".
func deskPaneOf(r hostRegistry, s ManagedSession) string {
	op, ok := r.Operations[receiptKey("create", s.CreateRequestID)]
	if !ok || op.Create == nil || op.SessionID != s.ID || !terminalproto.ValidID(op.Create.Desk.Pane) {
		return ""
	}
	return op.Create.Desk.Pane
}

// forgetDeskPane removes the pane's history files. A file already gone is not
// an error.
func (m *Manager) forgetDeskPane(pane string) error {
	if !terminalproto.ValidID(pane) {
		return terminalproto.ErrInvalid
	}
	var first error
	for _, kind := range []string{"bash", "zsh"} {
		if err := os.Remove(filepath.Join(deskHistoryDir(m.home), pane+"."+kind)); err != nil && !errors.Is(err, os.ErrNotExist) && first == nil {
			first = err
		}
	}
	return first
}

// ForgetDeskSession removes the history file of the desk pane the session was
// created for (End.ForgetDesk), now and once more after deskForgetAgain. A
// shell started for the same pane before then calls the second removal off,
// so its new history is kept. A session without a desk pane has nothing to
// remove.
func (m *Manager) ForgetDeskSession(ctx context.Context, sessionID string) error {
	if !terminalproto.ValidID(sessionID) {
		return terminalproto.ErrInvalid
	}
	// m.mu is held throughout, so a create for the same pane (which holds it
	// too) comes wholly before or after this removal.
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagerClosed
	}
	lock, err := m.registryLock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	r, err := m.loadRegistry()
	if err != nil {
		return err
	}
	s, ok := r.Sessions[sessionID]
	if !ok {
		return ErrSessionMissing
	}
	pane := deskPaneOf(r, s)
	if pane == "" {
		return nil
	}
	err = m.forgetDeskPane(pane)
	m.scheduleDeskForget(pane)
	return err
}

// scheduleDeskForget removes the pane's history again after deskForgetAgain,
// unless a new shell for the pane or Close calls it off first.
func (m *Manager) scheduleDeskForget(pane string) {
	m.deskMu.Lock()
	defer m.deskMu.Unlock()
	if old := m.deskForget[pane]; old != nil {
		old.Stop()
	}
	var t *time.Timer
	t = time.AfterFunc(deskForgetAgain, func() {
		m.deskMu.Lock()
		defer m.deskMu.Unlock()
		if m.deskForget[pane] != t {
			return // called off, or replaced by a later end
		}
		delete(m.deskForget, pane)
		_ = m.forgetDeskPane(pane)
	})
	m.deskForget[pane] = t
}

// cancelDeskForget calls off a pending second removal of the pane's history.
func (m *Manager) cancelDeskForget(pane string) {
	m.deskMu.Lock()
	defer m.deskMu.Unlock()
	if t := m.deskForget[pane]; t != nil {
		t.Stop()
		delete(m.deskForget, pane)
	}
}

// stopDeskForgets calls off every pending second removal (Close).
func (m *Manager) stopDeskForgets() {
	m.deskMu.Lock()
	defer m.deskMu.Unlock()
	for pane, t := range m.deskForget {
		t.Stop()
		delete(m.deskForget, pane)
	}
}

// pendingDeskForgets is how many second removals are still waiting.
func (m *Manager) pendingDeskForgets() int {
	m.deskMu.Lock()
	defer m.deskMu.Unlock()
	return len(m.deskForget)
}
