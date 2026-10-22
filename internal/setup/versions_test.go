package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReportedVersionFormats(t *testing.T) {
	for _, tc := range []struct{ agent, output, want string }{
		{"claude", "2.1.3 (Claude Code)\n", "2.1.3"},
		{"claude", "claude 2.1.3", "2.1.3"},
		{"codex", "codex-cli 0.77.0\n", "0.77.0"},
		{"opencode", "1.2.3\n", "1.2.3"},
		{"pi", "v0.58.1-rc.1", "0.58.1-rc.1"},
		{"cursor", "1.2.3\ncommit-hash\narm64", "1.2.3"},
		{"gemini", "gemini 0.22.0+build.1\r\n", "0.22.0+build.1"},
		{"codex", "SECRET=1.2.3", ""},
		{"codex", "warning\ncodex-cli 0.77.0", ""},
		{"codex", "codex-cli codex 0.77.0", ""},
		{"pi", "claude 2.1.3", ""},
		{"pi", "1.2", ""},
		{"pi", "1.2.3 extra-secret", ""},
		{"pi", strings.Repeat("9", 129) + ".2.3", ""},
	} {
		if got := reportedVersion(tc.agent, tc.output); got != tc.want {
			t.Errorf("%s %q: got %q, want %q", tc.agent, tc.output, got, tc.want)
		}
	}
}

func TestVersionProbeReportsPartialFailuresWithoutRawOutput(t *testing.T) {
	e := fixture(t)
	e.lookPath = func(name string) (string, error) { return filepath.Join(e.home, name), nil }
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	results := map[string]versionResult{
		"claude":   {output: []byte("2.1.3 (Claude Code)\n")},
		"codex":    {output: []byte("SECRET_FAILURE"), err: errors.New("SECRET_FAILURE")},
		"opencode": {err: context.DeadlineExceeded},
		"pi":       {output: []byte("SECRET_UNKNOWN")},
		"cursor":   {output: []byte("1.2.3"), overflow: true},
		"gemini":   {output: []byte("0.22.1\n")},
	}
	err = probeVersions(context.Background(), &r, func(ctx context.Context, path string) versionResult {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("probe has no deadline")
		}
		return results[filepath.Base(path)]
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"claude": "reported", "codex": "error", "opencode": "timeout", "pi": "unknown", "cursor": "unknown", "gemini": "reported"}
	for _, a := range r.Agents {
		if a.ApplicationVersion.State != want[a.ID] || a.ApplicationVersion.InstallMethod != "unknown" || a.Capabilities.Version != "command_report_only" {
			t.Fatalf("wrong observation: %+v", a)
		}
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var text bytes.Buffer
	if err := render(&text, r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data)+text.String(), "SECRET_") || strings.Contains(text.String(), "Executables are looked up on PATH, not run") {
		t.Fatal("raw probe output or contradictory limitation returned")
	}
	if !r.VersionProbes || r.SchemaVersion != 2 || findAgent(t, r, "claude").Components != nil {
		t.Fatal("version probe incorrectly opted into configuration reading")
	}
}

func TestUnavailableExecutablesAreNotProbed(t *testing.T) {
	r, err := discover(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	r.Agents[0].Executable.State = "error"
	r.Agents[1].Executable = Executable{State: "present", Path: "relative/codex"}
	err = probeVersions(context.Background(), &r, func(context.Context, string) versionResult {
		t.Fatal("unavailable executable was invoked")
		return versionResult{}
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Agents[0].ApplicationVersion.State != "unknown" || r.Agents[1].ApplicationVersion.State != "unknown" || r.Agents[2].ApplicationVersion.State != "not_detected" {
		t.Fatal("incorrect absent/unknown version states")
	}
}

func TestVersionProbeCancellationStopsFurtherCommands(t *testing.T) {
	e := fixture(t)
	e.lookPath = func(name string) (string, error) { return filepath.Join(e.home, name), nil }
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err = probeVersions(ctx, &r, func(ctx context.Context, _ string) versionResult {
		calls++
		cancel()
		return versionResult{err: ctx.Err()}
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancellation ignored: calls=%d err=%v", calls, err)
	}
}

func TestVersionOutputMemoryIsBounded(t *testing.T) {
	var b versionOutput
	for i := 0; i < 10; i++ {
		p := bytes.Repeat([]byte("x"), versionOutputLimit)
		if n, err := b.Write(p); err != nil || n != len(p) {
			t.Fatalf("failed to drain output: %d %v", n, err)
		}
	}
	if !b.overflow || len(b.data) != versionOutputLimit {
		t.Fatalf("unbounded output retained: %d", len(b.data))
	}
}

func shellProbe(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	path := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestActualVersionProcessHasFixedArgumentAndOmitsStderr(t *testing.T) {
	path := shellProbe(t, "[ \"$#\" = 1 ] && [ \"$1\" = --version ] || exit 4\nprintf 'codex-cli 1.2.3\\n'\nprintf 'PRIVATE_STDERR' >&2\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := runVersionProbe(ctx, path)
	if r.err != nil || string(r.output) != "codex-cli 1.2.3\n" || r.overflow {
		t.Fatalf("bad process result: %v %q", r.err, r.output)
	}
}

func TestActualVersionProcessTimesOut(t *testing.T) {
	path := shellProbe(t, "while :; do :; done\n")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r := runVersionProbe(ctx, path)
	if !errors.Is(r.err, context.DeadlineExceeded) {
		t.Fatalf("process was not cancelled: %v", r.err)
	}
}

func TestVersionCLIRequiresExplicitOptIn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	home := isolatedCLI(t)
	marker := filepath.Join(home, "invoked")
	path := filepath.Join(os.Getenv("PATH"), "claude")
	script := "#!/bin/sh\nprintf yes > '" + marker + "'\nprintf '2.1.3 (Claude Code)\\n'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, probe := range []bool{false, true} {
		args := []string{"scan", "--json"}
		if probe {
			args = append(args, "--probe-versions")
		}
		var out bytes.Buffer
		if err := Run(args, &out); err != nil {
			t.Fatal(err)
		}
		var report Report
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		_, statErr := os.Stat(marker)
		if (statErr == nil) != probe || (findAgent(t, report, "claude").ApplicationVersion != nil) != probe {
			t.Fatalf("incorrect opt-in behaviour: probe=%v", probe)
		}
		if probe && findAgent(t, report, "claude").ApplicationVersion.Version != "2.1.3" {
			t.Fatal("version missing from CLI report")
		}
	}
}
