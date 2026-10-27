// Package terminalhost owns the local terminal opt-in, private tmux socket,
// and independent agent/tmux services. It starts no shell in this increment.
package terminalhost

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrUnsafePath      = errors.New("terminal state directory is unsafe")
	ErrTmuxUnavailable = errors.New("tmux is unavailable or unsupported")
)

const maxSocketPath = 100

// Layout is the installation-specific private terminal state. Socket is a
// capability for the same OS user; never put it in a controller response.
type Layout struct {
	Dir    string
	Socket string
	Config string
	Logs   string
	base   string
}

// NewEnrollmentLayout prevents a replacement enrollment (or a second config
// beside the first) from sharing the previous enrollment's server/registry.
func NewEnrollmentLayout(configPath, agentID string) (Layout, error) {
	if !terminalproto.ValidID(agentID) {
		return Layout{}, ErrUnsafePath
	}
	base, err := NewLayout(configPath)
	if err != nil {
		return Layout{}, err
	}
	sum := sha256.Sum256([]byte(agentID))
	dir := filepath.Join(base.Dir, hex.EncodeToString(sum[:6]))
	l := Layout{Dir: dir, Socket: filepath.Join(dir, "tmux.sock"), Config: filepath.Join(dir, "tmux.conf"), Logs: filepath.Join(dir, "logs"), base: base.Dir}
	if len(l.Socket) > maxSocketPath {
		return Layout{}, fmt.Errorf("%w: enrollment socket path is too long; move its config to a shorter directory", ErrUnsafePath)
	}
	return l, nil
}

func (l Layout) String() string   { return "[terminal host layout]" }
func (l Layout) GoString() string { return "[terminal host layout]" }

// NewLayout derives private paths beside the agent configuration. The socket
// path must fit the shortest supported Unix socket limit.
func NewLayout(configPath string) (Layout, error) {
	absolute, err := filepath.Abs(configPath)
	if err != nil || !filepath.IsAbs(absolute) {
		return Layout{}, ErrUnsafePath
	}
	dir := filepath.Join(filepath.Dir(absolute), "terminal")
	layout := Layout{
		Dir: dir, Socket: filepath.Join(dir, "tmux.sock"),
		Config: filepath.Join(dir, "tmux.conf"), Logs: filepath.Join(dir, "logs"),
	}
	if len(layout.Socket) > maxSocketPath || !filepath.IsAbs(layout.Socket) || filepath.Dir(layout.Socket) != dir {
		return Layout{}, fmt.Errorf("%w: socket path is too long (%d bytes, limit %d); move the agent configuration to a shorter directory", ErrUnsafePath, len(layout.Socket), maxSocketPath)
	}
	return layout, nil
}

// Ensure creates or verifies the private layout. Unsafe ownership, symlinks,
// permissive modes, or non-socket entries fail closed instead of being repaired.
func Ensure(layout Layout) error {
	if layout.base != "" {
		if err := ensurePrivateDir(layout.base); err != nil {
			return err
		}
	}
	if err := ensurePrivateDir(layout.Dir); err != nil {
		return err
	}
	if err := ensurePrivateDir(layout.Logs); err != nil {
		return err
	}
	if err := ensureTmuxConfig(layout); err != nil {
		return err
	}
	if info, err := os.Lstat(layout.Socket); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("%w: %s is not a socket", ErrUnsafePath, layout.Socket)
		}
		if info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("%w: socket permissions are too open", ErrUnsafePath)
		}
		if err := ownedByCurrentUser(info); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ensurePrivateDir(dir string) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: %s is not a real directory", ErrUnsafePath, dir)
	}
	if info.Mode().Perm()&0077 != 0 || info.Mode().Perm()&0700 != 0700 {
		return fmt.Errorf("%w: %s must be private (chmod 700)", ErrUnsafePath, dir)
	}
	return ownedByCurrentUser(info)
}

func ensureTmuxConfig(layout Layout) error {
	info, err := os.Lstat(layout.Config)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, layout.Config)
		}
		if info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("%w: %s must be private (chmod 600)", ErrUnsafePath, layout.Config)
		}
		if err := ownedByCurrentUser(info); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return writePrivateFile(layout.Config, []byte(tmuxConfig))
}

// tmuxConfig is intentionally minimal. It never sources a personal file, loads
// plugins, or enables clipboard/escape-sequence passthrough.
const tmuxConfig = `set -g history-limit 5000
set -g default-terminal "screen-256color"
set -g escape-time 10
set -g focus-events off
set -g set-clipboard off
set -gq allow-passthrough off
set -g destroy-unattached off
set -g status off
`

func writePrivateFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".reet-private-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func CheckEnrollmentConfig(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("%w: enrollment configuration must be a private regular file", ErrUnsafePath)
	}
	return ownedByCurrentUser(info)
}

func cleanEnvValue(value string) bool {
	return value != "" && !strings.ContainsAny(value, "\x00\r\n")
}

// Check verifies an existing layout without creating or repairing anything.
func Check(layout Layout) error {
	dirs := []string{layout.Dir, layout.Logs}
	if layout.base != "" {
		dirs = append(dirs, layout.base)
	}
	for _, dir := range dirs {
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("%w: %s must be a private directory", ErrUnsafePath, dir)
		}
		if err := ownedByCurrentUser(info); err != nil {
			return err
		}
	}
	info, err := os.Lstat(layout.Config)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("%w: %s must be a private regular file", ErrUnsafePath, layout.Config)
	}
	return ownedByCurrentUser(info)
}
