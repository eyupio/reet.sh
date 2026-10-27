package agenthealth

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var osVersionOnce sync.Once
var cachedOSVersion string

// reportedOSVersion is informational and sent only to controllers that negotiated
// agents-v2. It never reads enrollment files or user environment values.
func reportedOSVersion(enabled bool) string {
	if !enabled {
		return ""
	}
	osVersionOnce.Do(func() { cachedOSVersion = readOSVersion() })
	return cachedOSVersion
}

func readOSVersion() string {
	var binary string
	var args []string
	switch runtime.GOOS {
	case "linux":
		binary = "/usr/bin/uname"
		args = []string{"-r"}
	case "darwin":
		binary = "/usr/bin/sw_vers"
		args = []string{"-productVersion"}
	default:
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, args...).Output()
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(out))
	if len(value) > 128 || !utf8.ValidString(value) {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return value
}
