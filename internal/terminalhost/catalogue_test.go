package terminalhost

import (
	"embed"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Captured help travels with standalone test binaries used by the tmux matrix.
//
//go:embed testdata/help/*.txt
var catalogueHelp embed.FS

func TestCatalogueHasSevenAgents(t *testing.T) {
	var ids []string
	for _, p := range Catalogue {
		ids = append(ids, p.ID)
	}
	if !reflect.DeepEqual(ids, []string{"claude", "codex", "opencode", "gemini", "pi", "cursor", "cline"}) {
		t.Fatal(ids)
	}
}

func TestOldControllerSeesOnlyFourTools(t *testing.T) {
	look := fakeLookPath("claude", "codex", "opencode", "gemini", "pi", "cursor-agent", "cline")
	want := []string{"claude", "codex", "gemini", "opencode"}
	if got := InstalledTools(look, false); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	// New launch modes remain unavailable until captured CLI help establishes
	// their prompt, model, and permission arguments.
	if got := InstalledTools(look, true); !reflect.DeepEqual(got, []string{"claude", "cline", "codex", "gemini", "opencode"}) {
		t.Fatal("unconfirmed preset advertised", got)
	}
}

func TestLaunchArgvEffortMapping(t *testing.T) {
	in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "codex", Effort: "high"}
	argv, err := LaunchArgv(in, fakeLookPath("codex"))
	if argv != nil || !errors.Is(err, ErrLaunchOptionUnsupported) || LaunchResultCode(err) != terminalproto.LaunchOptionUnsupported {
		t.Fatal(argv, err)
	}
}

func TestLaunchArgvYoloRefusedBeforeF(t *testing.T) {
	in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "codex", Yolo: true}
	argv, err := LaunchArgv(in, fakeLookPath("codex"))
	if argv != nil || !errors.Is(err, ErrYoloRefused) || LaunchResultCode(err) != terminalproto.YoloRefused {
		t.Fatal(argv, err)
	}
}

func TestUnconfirmedPresetsNeverLaunch(t *testing.T) {
	for _, tool := range []string{"pi", "cursor"} {
		in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: tool}
		argv, err := LaunchArgv(in, fakeLookPath("pi", "cursor-agent", "cline"))
		if argv != nil || !errors.Is(err, ErrLaunchUnavailable) {
			t.Fatal(tool, argv, err)
		}
	}
}

func TestPresetFlagsAppearInHelpText(t *testing.T) {
	// A standalone test binary must not depend on its launch directory.
	t.Chdir(t.TempDir())
	for _, p := range Catalogue {
		help, err := catalogueHelp.ReadFile("testdata/help/" + p.Binary + ".txt")
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			// The four shipped commands retain their pre-catalogue reviewed flags.
			// A new command without captured help must have no executable options.
			if !terminalproto.ValidTool(p.ID) && (p.DisabledReason == "" || p.ModelFlag != "" || p.PromptFlag != "" || len(p.SafeArgs) > 0 || len(p.EffortArgs) > 0 || len(p.YoloArgs) > 0) {
				t.Fatal("uncaptured preset enabled", p.ID)
			}
			continue
		}
		if !strings.Contains(string(help), "0.163.0") && p.ID == "codex" {
			t.Fatal("capture has no pinned version")
		}
		flags := []string{p.ModelFlag, p.PromptFlag}
		flags = append(flags, p.SafeArgs...)
		flags = append(flags, p.YoloArgs...)
		for _, args := range p.EffortArgs {
			flags = append(flags, args...)
		}
		for _, flag := range flags {
			if strings.HasPrefix(flag, "-") && !strings.Contains(string(help), flag) {
				t.Fatal(p.ID, "flag absent from captured help", flag)
			}
		}
	}
}

func TestCatalogueDoesNotEnableApprovalBypass(t *testing.T) {
	for _, p := range Catalogue {
		if len(p.YoloArgs) != 0 {
			t.Fatal("YOLO must remain deferred", p.ID)
		}
	}
}

func TestLaunchArgvConfirmedOptionMapping(t *testing.T) {
	original := launchPresets["codex"]
	defer func() { launchPresets["codex"] = original }()
	preset := original
	preset.safeArgs = []string{"--test-safe", "ask"}
	preset.effortArgs = map[string][]string{"high": {"--test-effort", "strong"}}
	launchPresets["codex"] = preset
	in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "codex", Effort: "high", Model: "m", Prompt: "run tests"}
	got, err := LaunchArgv(in, fakeLookPath("codex"))
	want := []string{"/opt/fake/bin/codex", "--test-safe", "ask", "--test-effort", "strong", "--model", "m", "--", "run tests"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
}

func TestClineAlwaysPassesAutoApproveFalse(t *testing.T) {
	for _, prompt := range []string{"", "fix the tests"} {
		in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "cline", Prompt: prompt}
		argv, err := LaunchArgv(in, fakeLookPath("cline"))
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for i, arg := range argv {
			if arg == "--auto-approve" {
				count++
				if i+1 >= len(argv) || argv[i+1] != "false" {
					t.Fatal(argv)
				}
			}
		}
		if count != 1 || !slicesContains(argv, "--tui") {
			t.Fatal("safe interactive defaults missing", argv)
		}
	}
}

func slicesContains(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestLaunchArgvClineConfirmedOptions(t *testing.T) {
	in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "cline", Model: "m", Effort: "high", Prompt: "fix the tests"}
	got, err := LaunchArgv(in, fakeLookPath("cline"))
	want := []string{"/opt/fake/bin/cline", "--auto-approve", "false", "--tui", "--thinking", "high", "--model", "m", "--", "fix the tests"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
	for _, prompt := range []string{"config", "update", "skill"} {
		in.Prompt = prompt
		if got, err := LaunchArgv(in, fakeLookPath("cline")); got != nil || !errors.Is(err, ErrLaunchUnavailable) {
			t.Fatal("subcommand-shaped prompt accepted", got, err)
		}
	}
}

func TestClinePromptCannotChangeApprovalMode(t *testing.T) {
	in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "cline", Prompt: "--auto-approve true"}
	if argv, err := LaunchArgv(in, fakeLookPath("cline")); argv != nil || !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal("leading flag prompt accepted", argv, err)
	}
	in.Prompt = "explain --auto-approve true"
	argv, err := LaunchArgv(in, fakeLookPath("cline"))
	want := []string{"/opt/fake/bin/cline", "--auto-approve", "false", "--tui", "--", in.Prompt}
	if err != nil || !reflect.DeepEqual(argv, want) {
		t.Fatal("prompt changed safe defaults", argv, err)
	}
}
