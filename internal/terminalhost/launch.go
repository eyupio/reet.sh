package terminalhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ErrLaunchUnavailable means a requested agent cannot be started on this
// machine: the tool is not in the built-in table, its binary is not on PATH, or
// the folder is refused. The connector reports it as terminalproto.LaunchFailed
// (see LaunchResultCode); it never falls back to a plain shell.
var ErrLaunchUnavailable = errors.New("the requested agent cannot be started here")

// These errors deliberately distinguish unsupported options from missing tools.
var ErrLaunchOptionUnsupported = errors.New("the requested launch option is not supported")
var ErrYoloRefused = errors.New("approval bypass is not available")

func launchUnavailable(reason string) error {
	return fmt.Errorf("%w: %s", ErrLaunchUnavailable, reason)
}

// LaunchResultCode maps launch refusals to their wire codes. It returns ""
// for other errors, preserving their existing mapping (normally Unavailable).
func LaunchResultCode(err error) terminalproto.ResultCode {
	if errors.Is(err, ErrYoloRefused) {
		return terminalproto.YoloRefused
	}
	if errors.Is(err, ErrLaunchOptionUnsupported) {
		return terminalproto.LaunchOptionUnsupported
	}
	if errors.Is(err, ErrLaunchUnavailable) {
		return terminalproto.LaunchFailed
	}
	return ""
}

// tmux hands a single-argument command to the shell (`sh -c`), so a binary
// path is restricted to characters no shell or tmux format treats specially.
// The leading '/' also makes it absolute.
var safeBinaryPath = regexp.MustCompile(`^/[A-Za-z0-9._+@/-]+$`)

func resolveBinary(p launchPreset, lookPath func(string) (string, error)) (string, bool) {
	if lookPath == nil {
		return "", false
	}
	path, err := lookPath(p.binary)
	if err != nil || !safeBinaryPath.MatchString(path) {
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
	if in.Yolo && len(p.yoloArgs) == 0 {
		return nil, ErrLaunchOptionUnsupported
	}
	args := p.safeArgs
	if in.Yolo {
		args = p.yoloArgs
	}
	argv := append([]string{path}, args...)
	if in.Resume {
		if len(p.resumeArgs) == 0 {
			return nil, ErrLaunchOptionUnsupported
		}
		argv = append(argv, p.resumeArgs...)
	}
	if in.Effort != "" {
		args, supported := p.effortArgs[in.Effort]
		if !supported || len(args) == 0 {
			return nil, ErrLaunchOptionUnsupported
		}
		argv = append(argv, args...)
	}
	if in.Model != "" {
		if p.modelFlag == "" {
			return nil, ErrLaunchOptionUnsupported
		}
		argv = append(argv, p.modelFlag, in.Model)
	}
	if in.Prompt != "" {
		if p.promptFlag != "" {
			argv = append(argv, p.promptFlag, in.Prompt)
		} else {
			// A command name is one word, so a prompt with whitespace can never
			// be dispatched as a subcommand.
			if terminalproto.PromptNeedsWords(in.Tool, in.Prompt) {
				return nil, launchUnavailable("single-word prompt")
			}
			argv = append(argv, "--", in.Prompt)
		}
	}
	return argv, nil
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

// tmuxLiteral escapes a value that tmux format-expands, such as new-session's
// -c directory: without it a folder named "a#(cmd)" runs cmd and "#S" or
// "#{...}" rewrite the path. Doubling every '#' makes tmux read it literally.
func tmuxLiteral(s string) string { return strings.ReplaceAll(s, "#", "##") }

// tmuxArgument protects one argument handed to a tmux command. tmux reads an
// argument that ends in ';' as a command separator, unless the character before
// that ';' is a backslash, which it then replaces by ';' (cmd-parse.y,
// cmd_parse_from_arguments, unchanged from 3.2 to 3.4). Inserting a backslash
// before a final ';' therefore yields the argument unchanged: "x;" -> "x\;"
// reaches the program as "x;", and "x\;" -> "x\\;" as "x\;". A ';' anywhere
// else is literal and left alone. This is independent of tmuxLiteral, which
// handles format expansion of -c.
func tmuxArgument(s string) string {
	if strings.HasSuffix(s, ";") {
		return s[:len(s)-1] + `\;`
	}
	return s
}
