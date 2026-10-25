package terminalhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ErrLaunchUnavailable means a requested agent cannot be started on this
// machine: the tool is not in the built-in table, its binary is not on PATH, or
// the folder is refused. The connector reports it as terminalproto.LaunchFailed
// (see LaunchResultCode); it never falls back to a plain shell.
var ErrLaunchUnavailable = errors.New("the requested agent cannot be started here")

func launchUnavailable(reason string) error {
	return fmt.Errorf("%w: %s", ErrLaunchUnavailable, reason)
}

// LaunchResultCode is the single mapping point from a host error to the
// launch_failed wire code. It returns "" for any other error, which keeps its
// existing mapping (normally Unavailable).
func LaunchResultCode(err error) terminalproto.ResultCode {
	if errors.Is(err, ErrLaunchUnavailable) {
		return terminalproto.LaunchFailed
	}
	return ""
}

// launchPreset is how one agent CLI is started in its interactive TUI. It is
// built into the connector; the controller only ever names the tool id.
type launchPreset struct {
	binary    string
	modelFlag string // passed as two arguments: modelFlag, model
	// promptFlag is passed as two arguments, promptFlag and prompt. Empty means
	// the prompt is the positional operand placed after a "--" separator.
	promptFlag string
}

// launchPresets is the allowlist. Every row was confirmed against the CLI's own
// help or official documentation on 2026-10-25; none uses an approval-bypass,
// auto-approve, print or other one-shot flag. A tool not listed here is never
// offered or started, whatever terminalproto.ValidTool accepts.
var launchPresets = map[string]launchPreset{
	// Claude Code 2.1.310, `claude --help` on the reference machine:
	//   "Usage: claude [options] [command] [prompt]" — "starts an interactive
	//   session by default, use -p/--print for non-interactive output";
	//   "--model <model>  Model for the current session".
	// Its parser (commander) ends options at "--", but still dispatches a
	// subcommand (purge, update, install, ...) when the first operand is a bare
	// command name, so positional prompts must contain whitespace (below).
	"claude": {binary: "claude", modelFlag: "--model"},
	// Codex CLI, https://learn.chatgpt.com/docs/developer-commands?surface=cli
	// (redirected from https://developers.openai.com/codex/cli/reference):
	//   "Running `codex` with no subcommand launches the interactive terminal UI";
	//   global flag "--model, -m  Override the model set in configuration";
	//   global "PROMPT  Optional text instruction to start the session".
	// Source: openai/codex codex-rs/tui/src/cli.rs `prompt: Option<String>`
	// (positional) flattened into the clap MultitoolCli; clap ends options at "--".
	"codex": {binary: "codex", modelFlag: "--model"},
	// OpenCode, https://opencode.ai/docs/cli/ : "opencode [project]  Start the
	// OpenCode terminal user interface", flags "--prompt  Prompt to use" and
	// "--model (-m)  Model to use in the form of provider/model". The positional
	// is a project path, so the prompt goes through --prompt.
	"opencode": {binary: "opencode", modelFlag: "--model", promptFlag: "--prompt"},
	// Gemini CLI, https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/cli-reference.md
	// and packages/cli/src/config/config.ts (yargs): "--prompt-interactive (-i)
	// Execute the provided prompt and continue in interactive mode", "--model
	// (-m)  Model". Not -p/--prompt, which is headless.
	"gemini": {binary: "gemini", modelFlag: "--model", promptFlag: "--prompt-interactive"},
}

// tmux hands a single-argument command to the shell (`sh -c`), so a binary
// path is restricted to characters no shell treats specially.
var safeBinaryPath = regexp.MustCompile(`^/[A-Za-z0-9._+@/-]+$`)

func resolveBinary(p launchPreset, lookPath func(string) (string, error)) (string, bool) {
	if lookPath == nil {
		return "", false
	}
	path, err := lookPath(p.binary)
	if err != nil || !filepath.IsAbs(path) || !safeBinaryPath.MatchString(path) {
		return "", false
	}
	return path, true
}

// LaunchArgv returns the program and arguments to start in.Tool, or nil for a
// plain shell. The model and prompt are each exactly one argument element; the
// prompt is guarded by "--" or by its flag so it can never be read as a flag.
func LaunchArgv(in terminalproto.Create, lookPath func(string) (string, error)) ([]string, error) {
	p, ok := launchPresets[in.Tool]
	if in.Tool != "" && !ok {
		return nil, launchUnavailable("unknown tool")
	}
	if in.Validate() != nil {
		return nil, terminalproto.ErrInvalid
	}
	if in.Tool == "" {
		return nil, nil
	}
	path, ok := resolveBinary(p, lookPath)
	if !ok {
		return nil, launchUnavailable("tool not installed")
	}
	argv := []string{path}
	if in.Model != "" {
		argv = append(argv, p.modelFlag, in.Model)
	}
	if in.Prompt != "" {
		if p.promptFlag != "" {
			argv = append(argv, p.promptFlag, in.Prompt)
		} else {
			// A command name is one word, so a prompt with whitespace can never
			// be dispatched as a subcommand.
			if !strings.ContainsAny(in.Prompt, " \n") {
				return nil, launchUnavailable("single-word prompt")
			}
			argv = append(argv, "--", in.Prompt)
		}
	}
	return argv, nil
}

// InstalledTools lists, sorted, the preset tool ids whose binary lookPath finds.
func InstalledTools(lookPath func(string) (string, error)) []string {
	out := []string{}
	for id, p := range launchPresets {
		if _, ok := resolveBinary(p, lookPath); ok {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// ResolveDir returns the working directory for a launch: home when dir is
// empty, otherwise dir with symlinks followed, which must be absolute, exist,
// be a directory and lie within home (also resolved). Failures wrap
// ErrLaunchUnavailable.
func ResolveDir(home, dir string) (string, error) {
	if !filepath.IsAbs(home) {
		return "", launchUnavailable("home is not absolute")
	}
	if dir == "" {
		return home, nil
	}
	if !filepath.IsAbs(dir) {
		return "", launchUnavailable("folder is not absolute")
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", launchUnavailable("home is unavailable")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", launchUnavailable("folder is unavailable")
	}
	rel, err := filepath.Rel(realHome, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", launchUnavailable("folder is outside home")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", launchUnavailable("folder is not a directory")
	}
	return resolved, nil
}
