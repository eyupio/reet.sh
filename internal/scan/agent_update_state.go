package scan

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/lifecycle"
)

// pendingUpdate is written beside the enrollment before the new binary
// replaces the old one. The new process reads it to prove that it started and
// can reach the controller; if it cannot, the previous binary goes back.
type pendingUpdate struct {
	JobID      string `json:"jobId"`
	From       string `json:"from"`
	FromCommit string `json:"fromCommit,omitempty"`
	To         string `json:"to"`
	Commit     string `json:"commit,omitempty"`
	Target     string `json:"target"`
	Previous   string `json:"previous"`
	Started    int64  `json:"started"`
	Attempts   int    `json:"attempts,omitempty"`
}

func pendingUpdatePath(config string) string { return config + ".update" }

func loadPendingUpdate(config string) (pendingUpdate, bool) {
	var p pendingUpdate
	b, err := os.ReadFile(pendingUpdatePath(config))
	if err != nil || json.Unmarshal(b, &p) != nil || p.JobID == "" || p.Previous == "" || p.Target == "" {
		return pendingUpdate{}, false
	}
	return p, true
}

func savePendingUpdate(config string, p pendingUpdate) error {
	return writePrivate(pendingUpdatePath(config), p)
}

func clearPendingUpdate(config string) {
	_ = os.Remove(pendingUpdatePath(config))
}

// parseAgentBuild reads the `reet <version> (<commit>)` line a binary prints.
func parseAgentBuild(out string) (version, commit string, ok bool) {
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 3 || fields[0] != "reet" {
		return "", "", false
	}
	return fields[1], strings.Trim(fields[2], "()"), true
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if commit == "" || strings.EqualFold(commit, "unknown") {
		return ""
	}
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// buildMatches reports whether the running process is the build the pending
// update installed. A stable build is identified by its tag; a dev build also
// by its source commit, because every dev build calls itself "dev".
func buildMatches(p pendingUpdate, version, commit string) bool {
	if !strings.EqualFold(strings.TrimSpace(version), strings.TrimSpace(p.To)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(p.To), "dev") {
		return true
	}
	want, got := shortCommit(p.Commit), shortCommit(commit)
	return want == "" || got == "" || strings.EqualFold(want, got)
}

func describeBuild(version, commit string) string {
	if strings.EqualFold(strings.TrimSpace(version), "dev") {
		if c := shortCommit(commit); c != "" {
			return "dev · " + c
		}
		return "dev"
	}
	return version
}

// updateHealthWindow is how long a new build may wait for a controller that is
// unreachable before the previous binary is put back. A controller answering
// with a refusal is the new build's fault and rolls back at once; a controller
// that cannot be reached may simply be restarting, so it gets five minutes.
const updateHealthWindow = 5 * time.Minute

// shouldRollback separates a controller that refused the new build from one
// that could not answer: a refusal will not improve by waiting, while a quiet
// controller may be restarting.
func shouldRollback(err error, started int64, at time.Time) bool {
	if strings.HasPrefix(err.Error(), "controller ") {
		return true
	}
	return at.Sub(time.UnixMilli(started)) >= updateHealthWindow
}

// undoPendingUpdate is used when the process could not re-exec after the new
// binary was written: put the old bytes back so a later service restart does
// not pick up an update nobody verified.
func undoPendingUpdate(config string) {
	pending, ok := loadPendingUpdate(config)
	if !ok {
		return
	}
	_ = lifecycle.ReplaceBinary(pending.Previous, pending.Target)
	clearPendingUpdate(config)
}
