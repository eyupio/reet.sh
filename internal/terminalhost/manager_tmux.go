//go:build linux || darwin

package terminalhost

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var paneID = regexp.MustCompile(`^%[0-9]+$`)

func integer(n int) string { return strconv.Itoa(n) }

type cappedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		b.overflow = true
		return 0, ErrOutputLimit
	}
	return b.Buffer.Write(p)
}
func (m *Manager) environment() []string {
	values := safeEnvironment()
	filtered := make([]string, 0, len(values)+2)
	for _, entry := range values {
		if !strings.HasPrefix(entry, "HOME=") && !strings.HasPrefix(entry, "TERM=") {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, "HOME="+m.home, "TERM=xterm-256color")
}

func (m *Manager) command(ctx context.Context, limit int, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit <= 0 {
		limit = 32 << 10
	}
	cmd := exec.CommandContext(ctx, m.tmux.Path, append([]string{"-S", m.layout.Socket}, args...)...)
	cmd.Env = m.environment()
	cmd.Dir = m.home
	var out cappedBuffer
	out.limit = limit
	cmd.Stdout = &out
	err := cmd.Run()
	if out.overflow {
		return nil, ErrOutputLimit
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrNotRunning
	}
	return out.Bytes(), nil
}

func (m *Manager) defaults(ctx context.Context) error {
	for _, args := range [][]string{{"set-option", "-g", "remain-on-exit", "on"}, {"set-option", "-g", "update-environment", ""}, {"set-option", "-g", "monitor-bell", "on"}} {
		if _, err := m.command(ctx, 0, args...); err != nil {
			return err
		}
	}
	return m.applyClipboard(ctx)
}

// applyClipboard follows the local clipboard switch on the running tmux
// server. OSC 52 reaches the attached browser only when set-clipboard is on;
// allow-passthrough stays off. It skips the command when the remembered value
// already matches, and forgets it after ErrNotRunning so a later server start
// applies the current switch.
func (m *Manager) applyClipboard(ctx context.Context) error {
	on := m.clipboardConsent == nil || m.clipboardConsent() == nil
	if m.clipboardSet && m.clipboardOn == on {
		return nil
	}
	value := "off"
	if on {
		value = "on"
	}
	if _, err := m.command(ctx, 0, "set-option", "-g", "set-clipboard", value); err != nil {
		if errors.Is(err, ErrNotRunning) {
			m.clipboardSet = false
		}
		return err
	}
	m.clipboardSet, m.clipboardOn = true, on
	return nil
}
func (m *Manager) sessionNames(ctx context.Context) (map[string]bool, error) {
	if !responsive(ctx, m.tmux, m.layout) {
		return nil, ErrNotRunning
	}
	out, err := m.command(ctx, 64<<10, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		// Only a successful enumeration proves absence. An empty -D server
		// returns a successful empty list; errors must retain uncertain state.
		return nil, err
	}
	names := map[string]bool{}
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name != "" {
			names[name] = true
		}
	}
	return names, nil
}

type paneState struct {
	id   string
	dead bool
	exit *int
}

func (m *Manager) panes(ctx context.Context, sessionID string) ([]paneState, error) {
	out, err := m.command(ctx, 64<<10, "list-panes", "-s", "-t", "="+m.name(sessionID), "-F", "#{pane_id}"+fieldSep+"#{pane_dead}"+fieldSep+"#{pane_dead_status}")
	if err != nil {
		return nil, err
	}
	panes := []paneState{}
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		fields := strings.Split(line, fieldSep)
		if len(fields) != 3 || !paneID.MatchString(fields[0]) || fields[1] != "0" && fields[1] != "1" {
			return nil, ErrNotRunning
		}
		p := paneState{id: fields[0], dead: fields[1] == "1"}
		if fields[2] != "" {
			code, err := strconv.Atoi(fields[2])
			if err != nil || code < 0 || code > 255 {
				return nil, ErrNotRunning
			}
			p.exit = &code
		}
		panes = append(panes, p)
	}
	return panes, nil
}

// PaneProcesses returns the process id tmux started in each pane of a
// session: the leader of the process group the pane's program runs in. A
// caller that ends the session reads it first, so it can wait for those
// groups to exit; End only asks tmux to close the panes.
func (m *Manager) PaneProcesses(ctx context.Context, sessionID string) ([]int, error) {
	if !terminalproto.ValidID(sessionID) {
		return nil, terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagerClosed
	}
	if !responsive(ctx, m.tmux, m.layout) {
		return nil, ErrNotRunning
	}
	out, err := m.command(ctx, 64<<10, "list-panes", "-s", "-t", "="+m.name(sessionID), "-F", "#{pane_pid}")
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 1 {
			return nil, ErrNotRunning
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// History is a separate bounded snapshot, not an atomic replay concatenated
// with a live stream. The caller must not persist or log the returned bytes.
func (m *Manager) History(ctx context.Context, sessionID string, lines int) ([]byte, error) {
	if lines < 1 || lines > terminalproto.MaxHistoryLines {
		return nil, terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.allowed(); err != nil {
		return nil, err
	}
	lock, err := m.registryLock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	r, err := m.loadRegistry()
	if err != nil {
		return nil, err
	}
	if err := m.reconcileLocked(ctx, &r); err != nil {
		return nil, err
	}
	s, ok := r.Sessions[sessionID]
	if !ok || s.State != terminalproto.Running || !paneID.MatchString(s.PaneID) {
		return nil, ErrSessionMissing
	}
	data, err := m.command(ctx, terminalproto.MaxQueuedOutputBytes, "capture-pane", "-p", "-t", s.PaneID, "-S", "-"+integer(lines))
	if err != nil {
		return nil, err
	}
	// Remove empty screen padding, then keep at most the requested line count.
	// The snapshot remains bytes; do not decode per data frame or persist it.
	data = bytes.TrimRight(data, "\n")
	remaining := bytes.Count(data, []byte{'\n'}) + 1 - lines
	for remaining > 0 {
		n := bytes.IndexByte(data, '\n')
		if n < 0 {
			break
		}
		data = data[n+1:]
		remaining--
	}
	if len(data) == 0 {
		return nil, nil
	}
	return append(data, '\n'), nil
}
