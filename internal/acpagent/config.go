//go:build linux || darwin

package acpagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// LaunchEnv is the connector-owned data an adapter may put in its private
// session environment.
type LaunchEnv struct {
	Model    string
	Password string
}

// ProbeVersion runs the adapter binary with a minimal environment and checks
// its reported version against the spec's floor. It returns the version that
// was found, or "" when the spec has no external version gate.
func ProbeVersion(ctx context.Context, binary string, spec Spec, dir, home string, base []string) (string, error) {
	if _, err := loginHome(spec, base); err != nil {
		return "", err
	}
	min, required := spec.minVersion()
	if !required {
		return "", nil
	}
	if binary == "" {
		return "", &VersionError{Agent: spec.displayName(), Required: spec.MinVersion}
	}
	// A cold start of the agent binary (or a busy machine) can outlast a short
	// probe, so a failed probe is tried once more before it is reported.
	found, err := probeOnce(ctx, binary, spec, dir, home, base, min)
	var verr *VersionError
	if errors.As(err, &verr) && verr.Reason == probeFailed && ctx.Err() == nil {
		found, err = probeOnce(ctx, binary, spec, dir, home, base, min)
	}
	return found, err
}

const probeFailed = "the version probe failed"

// busyStartTries and busyStartPause bound the wait for a binary that is busy
// being written: six tries, a little over a second in all.
const (
	busyStartTries = 5
	busyStartPause = 80 * time.Millisecond
)

func probeOnce(ctx context.Context, binary string, spec Spec, dir, home string, base []string, min semver) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	var stdout, stderr *cappedBuffer
	// A binary that something still holds open for writing cannot be run: the
	// kernel answers "text file busy". That is true for an instant after an
	// install or an update writes the file, and for as long as another process
	// forked while it was open takes to exec. It passes by itself, so the start
	// is tried again briefly; a command cannot be started twice, so each try
	// builds its own.
	for attempt := 0; ; attempt++ {
		cmd = exec.Command(binary, "--version")
		cmd.Dir = dir
		cmd.Env = minimalEnvironment(spec, home, base)
		cmd.Stdin = nil
		cmd.SysProcAttr = procAttr()
		stdout, stderr = &cappedBuffer{limit: 256}, &cappedBuffer{limit: 256}
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		err := cmd.Start()
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.ETXTBSY) || attempt >= busyStartTries || runCtx.Err() != nil {
			return "", &VersionError{Agent: spec.displayName(), Required: spec.MinVersion, Reason: "the binary could not be started"}
		}
		select {
		case <-runCtx.Done():
		case <-time.After(time.Duration(attempt+1) * busyStartPause):
		}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-runCtx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		waitErr = <-done
	}
	raw := strings.TrimSpace(stdout.String() + " " + stderr.String())
	failed := runCtx.Err() != nil || waitErr != nil
	invalid := failed || stdout.overflow
	if invalid || stderr.overflow {
		return "", &VersionError{Agent: spec.displayName(), Required: spec.MinVersion, Reason: probeFailed}
	}
	found, ok := extractSemver(raw)
	if !ok || !found.atLeast(min) {
		return "", &VersionError{Agent: spec.displayName(), Found: safeVersion(raw), Required: spec.MinVersion}
	}
	return found.String(), nil
}

func safeVersion(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no version"
	}
	if len(s) > 32 || !strings.Contains(s, ".") {
		return "an unknown version"
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r == '.' || r == '-' || r == '+') {
			return "an unknown version"
		}
	}
	return s
}

func serverPassword(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	var b [32]byte
	if _, err := io.ReadFull(random, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// baseEnvironment names the infrastructure a native coding agent needs. It
// deliberately excludes SSH agents, display servers, DBus addresses and every
// provider credential; a spec adds only the provider keys it was verified to
// read.
var baseEnvironment = map[string]bool{
	"PATH": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true,
	"XDG_STATE_HOME": true, "XDG_CACHE_HOME": true, "TMPDIR": true,
	"TMP": true, "TEMP": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true,
	"TZ": true, "USER": true, "LOGNAME": true,
}

// minimalEnvironment selects the infrastructure allowlist plus the adapter's
// provider allowlist, replaces HOME with the connector's home, and drops every
// other inherited value. A key reaches the child only when it is named here or
// in the spec, so an inherited OPENCODE_CONFIG or OPENCODE_PERMISSION cannot
// redirect the session.
func minimalEnvironment(spec Spec, home string, base []string) []string {
	allowed := make(map[string]bool, len(baseEnvironment)+len(spec.EnvAllowlist))
	for key := range baseEnvironment {
		allowed[key] = true
	}
	for _, key := range spec.EnvAllowlist {
		allowed[key] = true
	}
	values := map[string]string{}
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		valid := ok && key != "" && allowed[key]
		blockedXDG := spec.LoginHomeEnv != "" && strings.HasPrefix(key, "XDG_")
		if !valid || blockedXDG {
			continue
		}
		if strings.ContainsAny(value, "\x00\n\r") {
			continue
		}
		values[key] = value
	}
	if spec.LoginHomeEnv != "" {
		if path, err := loginHome(spec, base); err == nil {
			values[spec.LoginHomeEnv] = path
		}
	}
	values["HOME"] = home
	return envList(values)
}

// loginHome resolves a profile-owned route without reading credentials or
// changing the selected account. A malformed explicit path is refused, never
// replaced with a different account's default folder.
func loginHome(spec Spec, base []string) (string, error) {
	if spec.LoginHomeEnv == "" {
		return "", nil
	}
	home, path := "", ""
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "HOME":
			home = value
		case spec.LoginHomeEnv:
			path = value
		}
	}
	if path == "" {
		if filepath.IsAbs(home) && !strings.ContainsAny(home, "\x00\n\r") {
			path = filepath.Join(home, spec.LoginHomeDir)
		}
	}
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\n\r") {
		return "", fmt.Errorf("%s needs an absolute %s folder on that machine", spec.displayName(), spec.LoginHomeEnv)
	}
	return path, nil
}

// agentEnvironment is what an adapter process receives: the minimal inherited
// environment, then the adapter's own owned variables last so they always win.
func agentEnvironment(spec Spec, home string, base []string, launch LaunchEnv) []string {
	values := map[string]string{}
	for _, entry := range minimalEnvironment(spec, home, base) {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	if spec.ConfigEnv != nil {
		for key, value := range spec.ConfigEnv(launch) {
			if strings.ContainsAny(key, "=\x00\n\r") || strings.ContainsAny(value, "\x00\n\r") {
				continue
			}
			values[key] = value
		}
	}
	return envList(values)
}

func envList(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

type cappedBuffer struct {
	limit    int
	overflow bool
	buf      []byte
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(b.buf)+len(p) > b.limit {
		b.overflow = true
		remaining := b.limit - len(b.buf)
		if remaining > 0 {
			b.buf = append(b.buf, p[:remaining]...)
		}
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}
func (b *cappedBuffer) String() string { return string(b.buf) }
