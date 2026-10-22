package setup

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	versionOutputLimit = 8192
	versionTimeout     = 3 * time.Second
	noVersionProbeNote = "Executables are looked up on PATH, not run; application versions, installation methods and runtime health are unknown."
)

type VersionObservation struct {
	State         string `json:"state"`
	Version       string `json:"version,omitempty"`
	Source        string `json:"source"`
	InstallMethod string `json:"install_method"`
	Issue         string `json:"issue,omitempty"`
}

type versionResult struct {
	output   []byte
	overflow bool
	err      error
}

// ProbeVersions is explicitly opt-in: it runs the discovered executable with a
// fixed --version argument. Reported versions do not verify binary provenance.
func ProbeVersions(r *Report) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return probeVersions(ctx, r, runVersionProbe)
}

func probeVersions(ctx context.Context, r *Report, run func(context.Context, string) versionResult) error {
	r.SchemaVersion = 2
	r.VersionProbes = true
	notes := []string{
		"Version probes invoke discovered executables with --version, inheriting the current environment; commands may have their own side effects.",
		"Application versions are command reports, not verified package provenance or runtime health. Installation methods remain unknown.",
	}
	for _, note := range r.Limitations {
		if note != noVersionProbeNote {
			notes = append(notes, note)
		}
	}
	r.Limitations = notes
	for i := range r.Agents {
		if err := ctx.Err(); err != nil {
			return err
		}
		a := &r.Agents[i]
		a.Capabilities.Version = "command_report_only"
		v := &VersionObservation{State: "not_detected", Source: "executable --version", InstallMethod: "unknown"}
		a.ApplicationVersion = v
		if a.Executable.State == "missing" {
			continue
		}
		if a.Executable.State != "present" || !filepath.IsAbs(a.Executable.Path) {
			v.State, v.Issue = "unknown", "executable_unavailable"
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, versionTimeout)
		result := run(probeCtx, a.Executable.Path)
		cancel()
		if err := ctx.Err(); err != nil {
			return err
		}
		switch {
		case errors.Is(result.err, context.DeadlineExceeded):
			v.State, v.Issue = "timeout", "version_probe_timed_out"
		case result.err != nil:
			v.State, v.Issue = "error", "version_probe_failed"
		case result.overflow:
			v.State, v.Issue = "unknown", "version_output_too_large"
		default:
			v.Version = reportedVersion(a.ID, string(result.output))
			if v.Version == "" {
				v.State, v.Issue = "unknown", "version_output_unrecognised"
			} else {
				v.State = "reported"
			}
		}
	}
	return nil
}

type versionOutput struct {
	data     []byte
	overflow bool
}

func (b *versionOutput) Write(p []byte) (int, error) {
	remaining := versionOutputLimit - len(b.data)
	if len(p) > remaining {
		b.overflow = true
		b.data = append(b.data, p[:remaining]...)
	} else {
		b.data = append(b.data, p...)
	}
	return len(p), nil // Keep draining output without retaining unbounded data.
}

func runVersionProbe(ctx context.Context, path string) versionResult {
	var output versionOutput
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	cmd.WaitDelay = 200 * time.Millisecond
	configureVersionProcess(cmd)
	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) && cmd.Cancel != nil {
		_ = cmd.Cancel() // Best effort cleanup of a child retaining output pipes.
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return versionResult{output.data, output.overflow, err}
}

var versionPattern = regexp.MustCompile(`^v?([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)

func reportedVersion(agent, output string) string {
	line, _, _ := strings.Cut(output, "\n")
	line = strings.TrimSpace(line)
	prefixes := map[string][]string{
		"claude": {"claude "}, "codex": {"codex-cli ", "codex "},
		"opencode": {"opencode "}, "pi": {"pi "},
		"cursor": {"Cursor ", "cursor "}, "gemini": {"gemini "},
	}
	for _, prefix := range prefixes[agent] {
		if strings.HasPrefix(line, prefix) {
			line = strings.TrimPrefix(line, prefix)
			break
		}
	}
	if agent == "claude" {
		line = strings.TrimSuffix(line, " (Claude Code)")
	}
	if len(line) > 128 {
		return ""
	}
	match := versionPattern.FindStringSubmatch(line)
	if len(match) == 0 {
		return ""
	}
	return match[1]
}
