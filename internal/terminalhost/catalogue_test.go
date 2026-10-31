package terminalhost

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Captured help travels with standalone test binaries used by the tmux matrix.
//
//go:embed testdata/help/*.txt testdata/help/sources.json
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

func TestLaunchArgvYoloOption(t *testing.T) {
	in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "codex", Yolo: true}
	argv, err := LaunchArgv(in, fakeLookPath("codex"))
	want := []string{"/opt/fake/bin/codex", "--ask-for-approval", "never"}
	if err != nil || !reflect.DeepEqual(argv, want) {
		t.Fatal(argv, err)
	}
	in.Tool = "claude"
	if argv, err := LaunchArgv(in, fakeLookPath("claude")); argv != nil || !errors.Is(err, ErrLaunchOptionUnsupported) {
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
		checkCapturedVersion(t, p, string(help))
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

// capturedVersions maps a captured binary to the version recorded in
// sources.json. Codex predates sources.json and keeps its reviewed version.
func capturedVersions(t *testing.T) map[string]string {
	t.Helper()
	raw, err := catalogueHelp.ReadFile("testdata/help/sources.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sources []struct{ Capture, Version string }
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{"codex": "0.163.0"}
	for _, s := range doc.Sources {
		out[s.Capture] = s.Version
	}
	return out
}

// checkCapturedVersion requires the capture's first line (the CLI's own
// version output) to carry the version its source record pins.
func checkCapturedVersion(t *testing.T, p Preset, help string) {
	t.Helper()
	want := capturedVersions(t)[p.Binary]
	if want == "" {
		t.Fatal(p.ID, "capture has no pinned version in sources.json")
	}
	first, _, _ := strings.Cut(help, "\n")
	if !strings.Contains(first, want) {
		t.Fatalf("%s: captured version line %q does not match pinned %s", p.ID, first, want)
	}
}

// flagBlock returns the help text describing flag: from the flag to the next
// option line.
func isFlagRune(r byte) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
}

func flagBlock(help, flag string) string {
	for from := 0; ; {
		i := strings.Index(help[from:], flag)
		if i < 0 {
			return ""
		}
		i += from
		end := i + len(flag)
		from = end
		// Whole-token match only: "--thinking" is not "--thinking-budget".
		if i > 0 && isFlagRune(help[i-1]) || end < len(help) && isFlagRune(help[end]) {
			continue
		}
		rest := help[end:]
		for _, sep := range []string{"\n  -", "\n      --", "\n\nCommands:"} {
			if j := strings.Index(rest, sep); j >= 0 {
				rest = rest[:j]
			}
		}
		return rest
	}
}

// blockHasToken reports whether value appears in block as a whole token.
func blockHasToken(block, value string) bool {
	for _, tok := range strings.FieldsFunc(block, func(r rune) bool { return r > 127 || !isFlagRune(byte(r)) && r != '.' }) {
		if tok == value {
			return true
		}
	}
	return false
}

func TestHelpTokenMatching(t *testing.T) {
	help := "  --thinking-budget <n>   tokens: hig\n  --thinking <level>      none|low|medium|high|xhigh\n  -c, --cwd <path>\n"
	block := flagBlock(help, "--thinking")
	if !blockHasToken(block, "high") || blockHasToken(block, "hig") || blockHasToken(block, "hi") {
		t.Fatalf("token match wrong for %q", block)
	}
	if blockHasToken(flagBlock(help, "--thinking-budget"), "high") || flagBlock("  --thinking-budget x", "--thinking") != "" {
		t.Fatal("prefix flag matched")
	}
}

func TestPresetFlagValuesAppearInHelpText(t *testing.T) {
	for _, p := range Catalogue {
		if p.DisabledReason != "" {
			continue
		}
		raw, err := catalogueHelp.ReadFile("testdata/help/" + p.Binary + ".txt")
		if err != nil {
			continue
		}
		help := string(raw)
		check := func(args []string) {
			for i := 0; i+1 < len(args); i++ {
				if !strings.HasPrefix(args[i], "-") || strings.HasPrefix(args[i+1], "-") {
					continue
				}
				block, value := flagBlock(help, args[i]), args[i+1]
				boolean := (value == "true" || value == "false") && strings.Contains(block, "<boolean>")
				if !boolean && !blockHasToken(block, value) {
					t.Fatalf("%s: value %q for %s not described by captured help", p.ID, value, args[i])
				}
			}
		}
		check(p.SafeArgs)
		check(p.YoloArgs)
		for _, level := range []string{"low", "medium", "high"} {
			check(p.EffortArgs[level])
		}
	}
}

// bypassPair reports flag/value pairs that disable approval or sandboxing.
func bypassPair(flag, value string) bool {
	switch flag {
	case "--approval-mode":
		return value == "yolo" || value == "auto_edit" || value == "auto-edit"
	case "--ask-for-approval", "-a":
		return value == "never"
	case "--sandbox", "-s":
		return value == "danger-full-access"
	case "--permission-mode":
		return value == "acceptedits" || value == "bypasspermissions" || value == "dontask"
	}
	return false
}

// approvalBypass reports why args could switch off tool approval, or "".
func approvalBypass(args []string) string {
	for i, arg := range args {
		lower := strings.ToLower(arg)
		switch {
		case lower == "-y", lower == "--yolo", lower == "--auto", lower == "--full-auto", lower == "--force", lower == "-f":
			return arg
		case strings.HasPrefix(lower, "--dangerously"), strings.Contains(lower, "bypass"), strings.Contains(lower, "skip-permission"), strings.HasPrefix(lower, "--allow-all"), strings.HasPrefix(lower, "--trust-all"):
			return arg
		case i+1 < len(args) && bypassPair(lower, strings.ToLower(args[i+1])):
			return arg + " " + args[i+1]
		case lower == "--auto-approve" || strings.HasPrefix(lower, "--auto-approve="):
			value := strings.TrimPrefix(lower, "--auto-approve=")
			if lower == "--auto-approve" {
				value = ""
				if i+1 < len(args) {
					value = strings.ToLower(args[i+1])
				}
			}
			if value != "false" {
				return arg + " " + value
			}
		}
	}
	return ""
}

func TestApprovalBypassDenylist(t *testing.T) {
	for _, bad := range [][]string{{"--yolo"}, {"-y"}, {"--auto-approve", "true"}, {"--auto-approve"}, {"--auto-approve=true"}, {"--dangerously-skip-permissions"}, {"--full-auto"}, {"--approval-mode", "yolo"}, {"--ask-for-approval", "never"}, {"--sandbox", "danger-full-access"}, {"--permission-mode", "acceptEdits"}} {
		if approvalBypass(bad) == "" {
			t.Fatal("denylist missed", bad)
		}
	}
	if approvalBypass([]string{"--auto-approve", "false", "--tui", "--thinking", "high", "--approval-mode", "default", "--sandbox", "workspace-write"}) != "" {
		t.Fatal("safe cline defaults denied")
	}
}

func TestCatalogueKeepsApprovalBypassBehindYolo(t *testing.T) {
	for _, p := range Catalogue {
		if p.DisabledReason != "" {
			continue
		}
		groups := map[string][]string{"SafeArgs": p.SafeArgs}
		for level, args := range p.EffortArgs {
			groups["EffortArgs["+level+"]"] = args
		}
		for name, args := range groups {
			if hit := approvalBypass(args); hit != "" {
				t.Fatalf("%s %s enables approval bypass: %s", p.ID, name, hit)
			}
		}
		if len(p.YoloArgs) > 0 && approvalBypass(p.YoloArgs) == "" {
			t.Fatalf("%s YoloArgs do not bypass approvals", p.ID)
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

func TestClineYoloReplacesAutoApproveFalse(t *testing.T) {
	in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: "cline", Yolo: true, Prompt: "fix the tests"}
	got, err := LaunchArgv(in, fakeLookPath("cline"))
	want := []string{"/opt/fake/bin/cline", "--yolo", "--tui", "--", "fix the tests"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
	if slicesContains(got, "--auto-approve") {
		t.Fatal("safe flag kept beside YOLO", got)
	}
}

func TestYoloNeverWithoutFlag(t *testing.T) {
	for _, tc := range []struct {
		tool string
		args []string
	}{
		{"codex", []string{"--ask-for-approval", "never"}},
		{"opencode", []string{"--auto"}},
		{"cline", []string{"--yolo"}},
	} {
		in := terminalproto.Create{Title: "test", Cols: 80, Rows: 24, Tool: tc.tool}
		got, err := LaunchArgv(in, fakeLookPath(tc.tool))
		if err != nil {
			t.Fatal(tc.tool, err)
		}
		for _, arg := range tc.args {
			if slicesContains(got, arg) {
				t.Fatalf("%s emitted %s without YOLO: %v", tc.tool, arg, got)
			}
		}
	}
}

// TestControllerOptionTableMatchesCatalogue keeps the controller's pre-reservation
// checks in step with the presets this connector actually starts.
func TestControllerOptionTableMatchesCatalogue(t *testing.T) {
	for _, p := range Catalogue {
		enabled := p.DisabledReason == ""
		if terminalproto.ValidToolV2(p.ID) != enabled {
			t.Fatal(p.ID, "agents-v2 acceptance differs from the catalogue")
		}
		if !enabled {
			continue
		}
		effort := len(p.EffortArgs["low"]) > 0 && len(p.EffortArgs["medium"]) > 0 && len(p.EffortArgs["high"]) > 0
		if terminalproto.ToolTakesEffort(p.ID) != effort || terminalproto.ToolTakesModel(p.ID) != (p.ModelFlag != "") || terminalproto.ToolTakesYolo(p.ID) != (len(p.YoloArgs) > 0) {
			t.Fatal(p.ID, "controller option table differs from the catalogue")
		}
	}
}

func TestOnlyOpenCodeHasAConfirmedModelListing(t *testing.T) {
	if !ListsModels("opencode") {
		t.Fatal("OpenCode should list models")
	}
	for _, tool := range []string{"claude", "codex", "gemini", "cline", "pi", "cursor", "unknown"} {
		if ListsModels(tool) {
			t.Fatalf("%s should not run a model listing", tool)
		}
	}
	for _, p := range Catalogue {
		if p.ID != "opencode" || p.ModelList == nil {
			continue
		}
		if len(p.ModelList.Argv) != 1 || p.ModelList.Argv[0] != "models" {
			t.Fatalf("OpenCode argv drifted: %#v", p.ModelList.Argv)
		}
		want := map[string]bool{"OPENCODE_DISABLE_MODELS_FETCH=1": true, "OPENCODE_DISABLE_AUTOUPDATE=1": true, "NO_COLOR=1": true, "LANG=C.UTF-8": true, "CI=1": true}
		for _, env := range p.ModelList.Env {
			delete(want, env)
		}
		if len(want) != 0 {
			t.Fatalf("OpenCode listing env missing %v", want)
		}
		return
	}
	t.Fatal("OpenCode has no model listing row")
}

func TestLaunchArgvPrefersAManagedBinary(t *testing.T) {
	in := terminalproto.Create{Tool: "opencode", Title: "run", Cols: 80, Rows: 24}
	managed := "/home/me/.reet/software/opencode/versions/1.19.5/opencode"
	argv, err := launchArgv(in, fakeLookPath("opencode"), managed)
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) == 0 || argv[0] != managed {
		t.Fatalf("launch did not use the managed binary: %v", argv)
	}
}
