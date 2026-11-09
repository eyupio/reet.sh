package terminalhost

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A preset's OptionsFloor is the oldest CLI version the resume, session-id and effort arguments were measured against. An older CLI would
// answer with its own parse error (or ignore the effort override), so the launch is refused as launch_option_unsupported first.
var versionNumbers = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// parseToolVersion reads the first major.minor.patch in the output of `<tool> --version`. A pre-release suffix is ignored.
func parseToolVersion(output string) ([3]int, bool) {
	m := versionNumbers.FindStringSubmatch(output)
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
func checkToolVersion(in terminalproto.Create, binary string, read func(string) string) error {
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
	have, ok := parseToolVersion(read(binary))
	if ok && versionBelow(have, floor) {
		return ErrLaunchOptionUnsupported
	}
	return nil
}

const toolVersionTimeout = 5 * time.Second

// readToolVersion runs `<binary> --version` and returns what it printed, or "" when it cannot be run or does not finish in time.
func readToolVersion(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), toolVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
