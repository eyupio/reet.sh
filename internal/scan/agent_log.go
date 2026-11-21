//go:build linux || darwin

package scan

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"golang.org/x/sys/unix"
)

// The agent log is append-only and nothing else rotates it. When the agent
// starts and finds it over agentLogLimit, only the last agentLogKeep bytes
// stay.
const (
	agentLogLimit = 10 << 20
	agentLogKeep  = 1 << 20
)

// trimAgentLog cuts a log over limit down to its last keep bytes, starting at
// a whole line. It rewrites the file in place, so a process that already holds
// it open for appending carries on at the new end.
func trimAgentLog(path string, limit, keep int64) error {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() <= limit {
		return nil
	}
	tail := make([]byte, keep)
	n, err := file.ReadAt(tail, info.Size()-keep)
	if err != nil {
		return err
	}
	tail = tail[:n]
	if i := bytes.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	note := fmt.Sprintf("(log passed %d MiB; earlier lines were removed)\n", limit>>20)
	tail = append([]byte(note), tail...)
	if _, err = file.WriteAt(tail, 0); err != nil {
		return err
	}
	return file.Truncate(int64(len(tail)))
}

// trimOwnAgentLog trims the log this process already writes to: the one a
// service manager opened for it, or the one a daemon kept across an update.
// The launcher trims a new daemon's log before opening it.
func trimOwnAgentLog(config, agentID string) {
	out, err := os.Stdout.Stat()
	if err != nil || !out.Mode().IsRegular() {
		return
	}
	// Without append mode the next write would land past the new end.
	flags, err := unix.FcntlInt(os.Stdout.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_APPEND == 0 {
		return
	}
	var paths []string
	if dir, _, err := daemonPaths(config); err == nil {
		paths = append(paths, filepath.Join(dir, "agent.log"))
	}
	if layout, err := terminalhost.NewEnrollmentLayout(config, agentID); err == nil {
		paths = append(paths, filepath.Join(layout.Logs, "agent.log"))
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && os.SameFile(info, out) {
			_ = trimAgentLog(path, agentLogLimit, agentLogKeep)
			return
		}
	}
}
