package terminalhost

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// fakeLookPath resolves only the named binaries, each to /opt/fake/bin/<name>.
func fakeLookPath(installed ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, n := range installed {
			if n == name {
				return "/opt/fake/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestLaunchArgvPresets(t *testing.T) {
	look := fakeLookPath("claude", "codex", "opencode", "gemini")
	const model, prompt = "sonnet-4.5", "fix the failing tests"
	// Each row: neither, model, prompt, both.
	cases := map[string][][]string{
		"claude": {
			{"/opt/fake/bin/claude"},
			{"/opt/fake/bin/claude", "--model", model},
			{"/opt/fake/bin/claude", "--", prompt},
			{"/opt/fake/bin/claude", "--model", model, "--", prompt},
		},
		"codex": {
			{"/opt/fake/bin/codex"},
			{"/opt/fake/bin/codex", "--model", model},
			{"/opt/fake/bin/codex", "--", prompt},
			{"/opt/fake/bin/codex", "--model", model, "--", prompt},
		},
		"opencode": {
			{"/opt/fake/bin/opencode"},
			{"/opt/fake/bin/opencode", "--model", model},
			{"/opt/fake/bin/opencode", "--prompt", prompt},
			{"/opt/fake/bin/opencode", "--model", model, "--prompt", prompt},
		},
		"gemini": {
			{"/opt/fake/bin/gemini"},
			{"/opt/fake/bin/gemini", "--model", model},
			{"/opt/fake/bin/gemini", "--prompt-interactive", prompt},
			{"/opt/fake/bin/gemini", "--model", model, "--prompt-interactive", prompt},
		},
	}
	if len(cases) != len(launchPresets) {
		t.Fatal("every shipped preset needs an exact argv table", len(launchPresets))
	}
	for tool, want := range cases {
		inputs := []terminalproto.Create{
			{Title: "a", Cols: 80, Rows: 24, Tool: tool},
			{Title: "a", Cols: 80, Rows: 24, Tool: tool, Model: model},
			{Title: "a", Cols: 80, Rows: 24, Tool: tool, Prompt: prompt},
			{Title: "a", Cols: 80, Rows: 24, Tool: tool, Model: model, Prompt: prompt},
		}
		for i, in := range inputs {
			got, err := LaunchArgv(in, look)
			if err != nil || !reflect.DeepEqual(got, want[i]) {
				t.Fatalf("%s case %d: got %q, %v; want %q", tool, i, got, err, want[i])
			}
		}
	}
}

func TestLaunchArgvNeverUsesBypassOrPrintFlags(t *testing.T) {
	// The only flags a preset may emit. Anything else (approval bypass, print or
	// one-shot modes such as claude -p or gemini -p) needs a reviewed change here.
	allowed := map[string]bool{"--model": true, "--prompt-interactive": true, "--prompt": true, "": true}
	for tool, p := range launchPresets {
		if p.modelFlag != "--model" || !allowed[p.promptFlag] || p.binary != tool {
			t.Fatal("unreviewed preset flag", tool, p)
		}
		if tool == "opencode" && p.promptFlag != "--prompt" || tool == "gemini" && p.promptFlag != "--prompt-interactive" {
			t.Fatal("gemini's --prompt is headless; only opencode's --prompt is its TUI prompt", tool, p)
		}
	}
}

func TestLaunchArgvPlainShellAndRefusals(t *testing.T) {
	look := fakeLookPath("claude", "gemini")
	if argv, err := LaunchArgv(terminalproto.Create{Title: "a", Cols: 80, Rows: 24}, look); argv != nil || err != nil {
		t.Fatal("plain shell must have no argv", argv, err)
	}
	for _, in := range []terminalproto.Create{
		{Title: "a", Cols: 80, Rows: 24, Tool: "codex"},  // binary missing
		{Title: "a", Cols: 80, Rows: 24, Tool: "pi"},     // not an allowlisted tool
		{Title: "a", Cols: 80, Rows: 24, Tool: "Claude"}, // ids are exact
	} {
		if argv, err := LaunchArgv(in, look); argv != nil || !errors.Is(err, ErrLaunchUnavailable) {
			t.Fatal("unknown or missing tool must be unavailable", in.Tool, argv, err)
		}
	}
	relative := func(string) (string, error) { return "bin/claude", nil }
	if _, err := LaunchArgv(terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "claude"}, relative); !errors.Is(err, ErrLaunchUnavailable) {
		t.Fatal("a relative binary path must be refused", err)
	}
	unsafe := func(string) (string, error) { return "/opt/my tools/claude;x", nil }
	if _, err := LaunchArgv(terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "claude"}, unsafe); !errors.Is(err, ErrLaunchUnavailable) {
		t.Fatal("a binary path tmux could hand to a shell must be refused", err)
	}
	if _, err := LaunchArgv(terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: "claude"}, nil); !errors.Is(err, ErrLaunchUnavailable) {
		t.Fatal("nil lookPath must be unavailable", err)
	}
}

func TestLaunchArgvPromptIsOneArgumentNeverAFlag(t *testing.T) {
	look := fakeLookPath("claude", "codex", "opencode", "gemini")
	for tool := range launchPresets {
		// A leading '-' is refused outright, so it can never be read as a flag.
		for _, prompt := range []string{"-rf", "--help", "--dangerously-skip-permissions now"} {
			argv, err := LaunchArgv(terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: tool, Prompt: prompt}, look)
			if argv != nil || !errors.Is(err, terminalproto.ErrInvalid) {
				t.Fatal("leading dash prompt reached argv", tool, prompt, argv, err)
			}
		}
		for _, prompt := range []string{`a b "c"`, "run it -- --help please", "two\nlines; rm -rf ~", "it's $(whoami) `id`"} {
			argv, err := LaunchArgv(terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: tool, Model: "m", Prompt: prompt}, look)
			if err != nil || argv[len(argv)-1] != prompt {
				t.Fatal("prompt was not the final single argument", tool, prompt, argv, err)
			}
			count := 0
			for _, a := range argv {
				if strings.Contains(a, prompt) {
					count++
				}
			}
			if count != 1 || len(argv) != 5 {
				t.Fatal("prompt was split or duplicated", tool, argv)
			}
			if p := launchPresets[tool]; p.promptFlag == "" && argv[len(argv)-2] != "--" || p.promptFlag != "" && argv[len(argv)-2] != p.promptFlag {
				t.Fatal("prompt is not guarded by its separator or flag", tool, argv)
			}
		}
	}
}

func TestLaunchArgvRefusesSubcommandShapedPositionalPrompt(t *testing.T) {
	look := fakeLookPath("claude", "codex", "opencode", "gemini")
	// claude's parser dispatches a subcommand from the first operand even after
	// "--", so a one-word positional prompt such as "purge" or "update" would run
	// that subcommand instead of starting the TUI.
	for _, tool := range []string{"claude", "codex"} {
		for _, prompt := range []string{"purge", "update", "hello"} {
			if argv, err := LaunchArgv(terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: tool, Prompt: prompt}, look); argv != nil || !errors.Is(err, ErrLaunchUnavailable) {
				t.Fatal("single-word positional prompt must be refused", tool, prompt, argv, err)
			}
		}
	}
	// Flag-form prompts are never parsed as a subcommand.
	for _, tool := range []string{"opencode", "gemini"} {
		if _, err := LaunchArgv(terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Tool: tool, Prompt: "hello"}, look); err != nil {
			t.Fatal("flag-form single word prompt refused", tool, err)
		}
	}
}

func TestInstalledTools(t *testing.T) {
	if got := InstalledTools(fakeLookPath()); len(got) != 0 {
		t.Fatal("nothing installed", got)
	}
	if got := InstalledTools(fakeLookPath("gemini", "claude", "pi", "cursor")); !reflect.DeepEqual(got, []string{"claude", "gemini"}) {
		t.Fatal("installed tools", got)
	}
	got := InstalledTools(fakeLookPath("opencode", "gemini", "codex", "claude"))
	if !reflect.DeepEqual(got, []string{"claude", "codex", "gemini", "opencode"}) {
		t.Fatal("installed tools must be sorted", got)
	}
	if err := (terminalproto.Capabilities{ProtocolVersion: terminalproto.Version, OS: "linux", OSUser: "dev", Readiness: terminalproto.Ready, TerminalEnabled: true, TerminalControl: true, Launch: true, Tools: got}).Validate(); err != nil {
		t.Fatal("installed tools must be valid capabilities", err)
	}
	if got := InstalledTools(func(string) (string, error) { return "relative/claude", nil }); len(got) != 0 {
		t.Fatal("a relative binary is not installed", got)
	}
	if got := InstalledTools(nil); len(got) != 0 {
		t.Fatal("nil lookPath", got)
	}
}

func TestResolveDir(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sub := filepath.Join(home, "work", "site")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{sub, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sub, filepath.Join(home, "inside-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "escape-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(home, "parent-link")); err != nil {
		t.Fatal(err)
	}
	// A sibling sharing the home's prefix must not count as inside it.
	if err := os.MkdirAll(home+"-other", 0o700); err != nil {
		t.Fatal(err)
	}
	realHome, _ := filepath.EvalSymlinks(home)
	realSub, _ := filepath.EvalSymlinks(sub)

	ok := map[string]string{
		"":                                 home,
		home:                               realHome,
		home + "/":                         realHome,
		sub:                                realSub,
		sub + "/":                          realSub,
		filepath.Join(home, "inside-link"): realSub,
		home + "/work/../work/site":        realSub,
	}
	for dir, want := range ok {
		got, err := ResolveDir(home, dir)
		if err != nil || got != want {
			t.Fatalf("ResolveDir(%q) = %q, %v; want %q", dir, got, err, want)
		}
	}
	for _, dir := range []string{
		filepath.Join(home, "escape-link"),
		filepath.Join(home, "escape-link") + "/",
		filepath.Join(home, "parent-link"),
		filepath.Join(home, "notes.txt"),
		filepath.Join(home, "missing"),
		"work/site",
		"./work",
		outside,
		root,
		home + "-other",
		"/",
	} {
		if got, err := ResolveDir(home, dir); err == nil || !errors.Is(err, ErrLaunchUnavailable) {
			t.Fatalf("ResolveDir(%q) = %q, %v; want ErrLaunchUnavailable", dir, got, err)
		}
	}
	if _, err := ResolveDir("relative-home", ""); !errors.Is(err, ErrLaunchUnavailable) {
		t.Fatal("relative home must be refused", err)
	}
}

func TestLaunchResultCode(t *testing.T) {
	for _, err := range []error{ErrLaunchUnavailable, launchUnavailable("folder"), errors.Join(errors.New("x"), ErrLaunchUnavailable)} {
		if LaunchResultCode(err) != terminalproto.LaunchFailed {
			t.Fatal("launch errors map to launch_failed", err)
		}
	}
	if LaunchResultCode(ErrNotRunning) != "" || LaunchResultCode(nil) != "" {
		t.Fatal("other errors are not launch failures")
	}
}

func TestTmuxLiteral(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "",
		"/home/dev/site":         "/home/dev/site",
		"/h/a#(touch x)#S":       "/h/a##(touch x)##S",
		"##":                     "####",
		"/h/#{session_name} é$x": "/h/##{session_name} é$x",
	} {
		if got := tmuxLiteral(in); got != want {
			t.Fatalf("tmuxLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveDirRelativeThatExistsFromCwd(t *testing.T) {
	// "." always exists relative to the cwd; a cwd-relative folder inside home
	// must still be refused for being relative.
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(home)
	for _, dir := range []string{".", "work", "./work"} {
		if got, err := ResolveDir(home, dir); !errors.Is(err, ErrLaunchUnavailable) {
			t.Fatalf("relative %q resolved to %q, %v", dir, got, err)
		}
	}
}

func TestResolveDirHomeIsASymlink(t *testing.T) {
	root := t.TempDir()
	realHome := filepath.Join(root, "real-home")
	if err := os.MkdirAll(filepath.Join(realHome, "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home-link")
	if err := os.Symlink(realHome, home); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(realHome, "work"))
	for _, dir := range []string{filepath.Join(home, "work"), filepath.Join(realHome, "work")} {
		if got, err := ResolveDir(home, dir); err != nil || got != want {
			t.Fatalf("ResolveDir(%q, %q) = %q, %v; want %q", home, dir, got, err, want)
		}
	}
}

// The controller refuses single-word prompts with the shared rule before they
// are sent; it must cover exactly the presets whose prompt is positional.
func TestPromptNeedsWordsMatchesPositionalPresets(t *testing.T) {
	for tool, p := range launchPresets {
		if terminalproto.PromptNeedsWords(tool, "purge") != (p.promptFlag == "") {
			t.Fatal("shared single-word rule disagrees with the preset", tool)
		}
	}
}

func TestTmuxArgument(t *testing.T) {
	for in, want := range map[string]string{
		"":              "",
		"new-session":   "new-session",
		"a;b c":         "a;b c",
		";a":            ";a",
		"a;":            `a\;`,
		";":             `\;`,
		`a\;`:           "a\\\\;", // a, two backslashes, ';' reaches the program as `a\;`
		"x;;":           `x;\;`,
		`a\`:            `a\`,
		"/home/d/work;": `/home/d/work\;`,
	} {
		if got := tmuxArgument(in); got != want {
			t.Fatalf("tmuxArgument(%q) = %q, want %q", in, got, want)
		}
	}
}
