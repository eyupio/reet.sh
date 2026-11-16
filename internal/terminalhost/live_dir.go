package terminalhost

import (
	"context"
	"regexp"
	"strings"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// managedName is the tmux session name Manager.name makes for a session id.
var managedName = regexp.MustCompile(`^reet-[0-9a-f]{32}$`)

// liveDirs reads every session's live working folder in one tmux call, keyed
// by tmux session name (Manager.name of the session id). It is its own call,
// separate from allPanes, whose format it leaves alone. A session with several
// panes reports its first. A row whose folder is not an absolute, plain path
// is left out, as is one whose folder held a newline. A failure returns an
// empty map: a caller reading folders must never fail the sweep over them.
func (m *Manager) liveDirs(ctx context.Context) (map[string]string, error) {
	// The session name comes first and the folder last, split off with a
	// limit, so a folder holding the separator keeps it.
	format := "#{session_name}" + fieldSep + "#{pane_current_path}"
	out, err := m.command(ctx, 1<<20, "list-panes", "-a", "-F", format)
	if err != nil {
		return map[string]string{}, err
	}
	return parseLiveDirs(out), nil
}

// parseLiveDirs reads liveDirs' listing. A tmux that prints a newline in a
// folder as it is splits that row in two: the line after it does not start
// with a managed session name, so the row before it is dropped as broken.
func parseLiveDirs(out []byte) map[string]string {
	type row struct {
		name, dir string
		broken    bool
	}
	var rows []row
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		name, dir, ok := strings.Cut(line, fieldSep)
		if !ok || !managedName.MatchString(name) {
			if len(rows) > 0 {
				rows[len(rows)-1].broken = true
			}
			continue
		}
		rows = append(rows, row{name: name, dir: dir})
	}
	dirs := map[string]string{}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.name] {
			continue
		}
		seen[r.name] = true
		if !r.broken && terminalproto.ValidDir(r.dir) {
			dirs[r.name] = r.dir
		}
	}
	return dirs
}

// noteLiveDirsLocked reads the live folder of every session that still has
// its tmux session and keeps it in memory. It reports whether any folder
// changed. A failed read changes nothing and is not an error. The caller
// holds m.mu.
func (m *Manager) noteLiveDirsLocked(ctx context.Context, r *hostRegistry) bool {
	for id := range m.liveDir {
		if _, ok := r.Sessions[id]; !ok {
			delete(m.liveDir, id)
		}
	}
	live := false
	for _, s := range r.Sessions {
		live = live || s.State.ReservesQuota()
	}
	if !live {
		return false
	}
	read := m.readLiveDirs
	if read == nil {
		read = m.liveDirs
	}
	dirs, err := read(ctx)
	if err != nil {
		return false
	}
	changed := false
	for id, s := range r.Sessions {
		if !s.State.ReservesQuota() {
			continue
		}
		dir, ok := dirs[m.name(id)]
		if ok && dir != m.liveDir[id] {
			m.liveDir[id] = dir
			changed = true
		}
	}
	return changed
}

// signalLocked wakes Changed's readers without a registry write. The caller
// holds m.mu.
func (m *Manager) signalLocked() {
	if m.closed {
		return
	}
	close(m.changed)
	m.changed = make(chan struct{})
}
