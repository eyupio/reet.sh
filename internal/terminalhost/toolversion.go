package terminalhost

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A preset's OptionsFloor is the oldest CLI version the resume, session-id and effort arguments were measured against. An older CLI would
// answer with its own parse error (or ignore the effort override), so the launch is refused as launch_option_unsupported first.
var versionNumbers = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// preRelease matches a version with a suffix, such as 0.156.1-alpha.2: older than the release it leads to.
var preRelease = regexp.MustCompile(`\d+\.\d+\.\d+-[0-9A-Za-z]`)

// parseToolVersion reads the major.minor.patch of the first line of `<tool> --version` output that has one; the suffix of a pre-release is
// left to preRelease.
func parseToolVersion(output string) ([3]int, bool) {
	m := versionNumbers.FindStringSubmatch(firstVersionLine(output))
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}

// firstVersionLine is the line of output that names the version: the first one with a number triple, so blank lines and notices without one
// are skipped.
func firstVersionLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if versionNumbers.MatchString(line) {
			return line
		}
	}
	return ""
}

func versionBelow(have, floor [3]int) bool {
	for i := range have {
		if have[i] != floor[i] {
			return have[i] < floor[i]
		}
	}
	return false
}

// checkToolVersion refuses a launch that asks for an option the machine's CLI is older than the measured floor for. Launches without such an
// option, tools without a floor, and a version that cannot be read are never refused: the CLI's own answer beats a guess.
func checkToolVersion(ctx context.Context, in terminalproto.Create, binary string, read func(context.Context, string) string) error {
	p, ok := launchPresets[in.Tool]
	if !ok || p.optionsFloor == "" || read == nil {
		return nil
	}
	if !in.Resume && in.LaunchSession == "" && in.Effort == "" {
		return nil
	}
	floor, ok := parseToolVersion(p.optionsFloor)
	if !ok {
		return nil
	}
	out := read(ctx, binary)
	have, ok := parseToolVersion(out)
	if ok && (versionBelow(have, floor) || (have == floor && preRelease.MatchString(firstVersionLine(out)))) {
		return ErrLaunchOptionUnsupported
	}
	return nil
}

// toolVersionTimeout bounds `--version`; a variable so a test can shorten it.
var toolVersionTimeout = 3 * time.Second

// readToolVersion runs `<binary> --version` and returns what it printed, or "" when it cannot be run, the request ends, or it does not finish
// in time. A timeout or a cancelled request stops the command and everything it started.
func readToolVersion(ctx context.Context, binary string) string {
	ctx, cancel := context.WithTimeout(ctx, toolVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	killGroupOnCancel(cmd)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
