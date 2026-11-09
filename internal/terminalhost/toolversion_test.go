package terminalhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestParseToolVersion(t *testing.T) {
	for out, want := range map[string][3]int{
		"codex-cli 0.166.0\n":                          {0, 166, 0},
		"codex-cli 0.156.1":                            {0, 156, 1},
		"codex-cli 0.170.0-alpha.3":                    {0, 170, 0},
		"2.1.319 (Claude Code)":                        {2, 1, 319},
		"warning: update available\ncodex-cli 1.2.3\n": {1, 2, 3},
	} {
		if got, ok := parseToolVersion(out); !ok || got != want {
			t.Errorf("%q: got %v %v, want %v", out, got, ok, want)
		}
	}
	for _, out := range []string{"", "codex", "version unknown", "1.2"} {
		if _, ok := parseToolVersion(out); ok {
			t.Errorf("%q parsed as a version", out)
		}
	}
}

func TestVersionBelow(t *testing.T) {
	for _, c := range []struct {
		have, floor [3]int
		below       bool
	}{
		{[3]int{0, 155, 9}, [3]int{0, 156, 1}, true},
		{[3]int{0, 156, 0}, [3]int{0, 156, 1}, true},
		{[3]int{0, 156, 1}, [3]int{0, 156, 1}, false},
		{[3]int{0, 166, 0}, [3]int{0, 156, 1}, false},
		{[3]int{1, 0, 0}, [3]int{0, 999, 999}, false},
	} {
		if got := versionBelow(c.have, c.floor); got != c.below {
			t.Errorf("%v below %v: got %v", c.have, c.floor, got)
		}
	}
}

// The floor protects the options that need a newer CLI (resume, a session id, effort). Plain launches, YOLO, a model and a prompt never run `--version`.
func TestCheckToolVersion(t *testing.T) {
	old := func(context.Context, string) string { return "codex-cli 0.150.0" }
	current := func(context.Context, string) string { return "codex-cli 0.166.0" }
	unreadable := func(context.Context, string) string { return "" }
	never := func(context.Context, string) string {
		t.Fatal("version read for a launch with no gated option")
		return ""
	}
	base := terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "codex"}
	with := func(f func(*terminalproto.Create)) terminalproto.Create { c := base; f(&c); return c }
	gated := map[string]terminalproto.Create{
		"resume":  with(func(c *terminalproto.Create) { c.Resume = true }),
		"session": with(func(c *terminalproto.Create) { c.LaunchSession = "0199-abc" }),
		"effort":  with(func(c *terminalproto.Create) { c.Effort = "high" }),
	}
	for name, in := range gated {
		if err := checkToolVersion(context.Background(), in, "/x/codex", old); !errors.Is(err, ErrLaunchOptionUnsupported) || LaunchResultCode(err) != terminalproto.LaunchOptionUnsupported {
			t.Errorf("%s on an old CLI: %v", name, err)
		}
		if err := checkToolVersion(context.Background(), in, "/x/codex", current); err != nil {
			t.Errorf("%s on a current CLI: %v", name, err)
		}
		// A version that cannot be read does not block: the CLI's own error is the better answer than a guess.
		if err := checkToolVersion(context.Background(), in, "/x/codex", unreadable); err != nil {
			t.Errorf("%s with no readable version: %v", name, err)
		}
	}
	for _, in := range []terminalproto.Create{base, with(func(c *terminalproto.Create) { c.Yolo = true; c.Model = "gpt-6"; c.Prompt = "hi" })} {
		if err := checkToolVersion(context.Background(), in, "/x/codex", never); err != nil {
			t.Errorf("an ungated launch was refused: %v", err)
		}
	}
	// A tool with no floor is never asked.
	if err := checkToolVersion(context.Background(), terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "opencode", Resume: true}, "/x/opencode", never); err != nil {
		t.Errorf("a tool without a floor was refused: %v", err)
	}
}

func TestManagerRefusesGatedCodexOptionOnOldCLI(t *testing.T) {
	m, opts := managerFixture(t)
	agent, _ := fakeAgent(t)
	m.lookPath = func(name string) (string, error) {
		if name == "codex" {
			return agent, nil
		}
		return "", errors.New("not found")
	}
	calls := recordingTmux(t, m)
	work := filepath.Join(opts.Home, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	m.toolVersion = func(context.Context, string) string { return "codex-cli 0.150.0" }
	in := terminalproto.Create{Title: "Codex", Cols: 100, Rows: 30, Tool: "codex", Dir: work, Resume: true}
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), in); !errors.Is(err, ErrLaunchOptionUnsupported) {
		t.Fatalf("old CLI resume: %v", err)
	}
	if n := len(newSessionCalls(calls())); n != 0 {
		t.Fatalf("a refused launch started %d sessions", n)
	}
	m.toolVersion = func(context.Context, string) string { return "codex-cli 0.166.0" }
	if _, err := m.Create(context.Background(), testRequestID(2), testSessionID(2), in); err != nil {
		t.Fatalf("current CLI resume: %v", err)
	}
}

func TestReadToolVersionRunsTheBinary(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"codex-cli 0.12.3\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := readToolVersion(context.Background(), script); got != "codex-cli 0.12.3" {
		t.Fatalf("got %q", got)
	}
	if got := readToolVersion(context.Background(), filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("a binary that cannot run read as %q", got)
	}
}

// A pre-release of the floor itself is older than the floor, and a notice printed on stdout before the version line must not be read as the version.
func TestCheckToolVersionPreReleaseOfTheFloor(t *testing.T) {
	in := terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "codex", Resume: true}
	say := func(out string) func(context.Context, string) string {
		return func(context.Context, string) string { return out }
	}
	if err := checkToolVersion(context.Background(), in, "/x/codex", say("codex-cli 0.156.1-alpha.2")); !errors.Is(err, ErrLaunchOptionUnsupported) {
		t.Errorf("a pre-release of the floor passed: %v", err)
	}
	if err := checkToolVersion(context.Background(), in, "/x/codex", say("codex-cli 0.156.2-alpha.2")); err != nil {
		t.Errorf("a pre-release after the floor was refused: %v", err)
	}
	if err := checkToolVersion(context.Background(), in, "/x/codex", say("Node 18.0.0 is deprecated\ncodex-cli 0.166.0")); err != nil {
		t.Errorf("a notice before the version line was read as the version: %v", err)
	}
}
