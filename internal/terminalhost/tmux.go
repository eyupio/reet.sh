package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	minTmuxMajor = 3
	minTmuxMinor = 3
)

var ErrNotRunning = errors.New("terminal tmux service is not running")

var sessionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type Tmux struct {
	Path    string
	Version string
}

func FindTmux(ctx context.Context) (Tmux, error) {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return Tmux{}, ErrTmuxUnavailable
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Tmux{}, ErrTmuxUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "-V").Output()
	if err != nil {
		return Tmux{}, ErrTmuxUnavailable
	}
	version := strings.TrimSpace(string(output))
	major, minor, err := parseTmuxVersion(version)
	if err != nil || !supportedTmux(major, minor) {
		return Tmux{}, fmt.Errorf("%w: tmux 3.3 or newer is required", ErrTmuxUnavailable)
	}
	return Tmux{Path: path, Version: version}, nil
}

func supportedTmux(major, minor int) bool {
	return major > minTmuxMajor || major == minTmuxMajor && minor >= minTmuxMinor
}

func parseTmuxVersion(value string) (int, int, error) {
	fields := strings.Fields(value)
	if len(fields) != 2 || fields[0] != "tmux" {
		return 0, 0, ErrTmuxUnavailable
	}
	raw := strings.TrimPrefix(fields[1], "next-")
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) < 2 {
		return 0, 0, ErrTmuxUnavailable
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, ErrTmuxUnavailable
	}
	minorRaw := parts[1]
	for len(minorRaw) > 0 && (minorRaw[len(minorRaw)-1] < '0' || minorRaw[len(minorRaw)-1] > '9') {
		minorRaw = minorRaw[:len(minorRaw)-1]
	}
	minor, err := strconv.Atoi(minorRaw)
	if err != nil {
		return 0, 0, ErrTmuxUnavailable
	}
	return major, minor, nil
}

type Session struct {
	Name     string `json:"name"`
	Created  int64  `json:"created"`
	Attached bool   `json:"attached"`
	Windows  int    `json:"windows"`
}

func SocketActive(layout Layout) bool {
	info, err := os.Lstat(layout.Socket)
	return err == nil && info.Mode()&os.ModeSocket != 0 && info.Mode().Perm()&0077 == 0 && ownedByCurrentUser(info) == nil
}

func Sessions(ctx context.Context, tmux Tmux, layout Layout) ([]Session, error) {
	if !SocketActive(layout) {
		return []Session{}, ErrNotRunning
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, tmux.Path, "-S", layout.Socket, "list-sessions", "-F", "#{session_name}\t#{session_created}\t#{session_attached}\t#{session_windows}")
	output, err := command.Output()
	if err != nil {
		if len(output) == 0 {
			return []Session{}, ErrNotRunning
		}
		return nil, ErrNotRunning
	}
	if len(output) > 1<<20 {
		return nil, ErrTmuxUnavailable
	}
	sessions := []Session{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) != 4 || !sessionName.MatchString(fields[0]) {
			continue
		}
		created, _ := strconv.ParseInt(fields[1], 10, 64)
		attached, _ := strconv.Atoi(fields[2])
		windows, _ := strconv.Atoi(fields[3])
		sessions = append(sessions, Session{Name: fields[0], Created: created * 1000, Attached: attached > 0, Windows: windows})
	}
	return sessions, nil
}

// EndSession ends one local tmux session. It is a local administrative action
// and does not create a controller operation.
func EndSession(ctx context.Context, tmux Tmux, layout Layout, name string) error {
	if !sessionName.MatchString(name) {
		return errors.New("invalid local session name")
	}
	if !SocketActive(layout) {
		return ErrNotRunning
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, tmux.Path, "-S", layout.Socket, "kill-session", "-t", "="+name).Run(); err != nil {
		return errors.New("could not end the local tmux session")
	}
	return nil
}
