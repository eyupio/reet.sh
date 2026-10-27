package agenthealth

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
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

const osReleasePath = "/etc/os-release"

func readOSVersion() string {
	if runtime.GOOS != "linux" {
		return commandOSVersion()
	}
	return readOSVersionFrom(osReleasePath, commandOSVersion)
}

// readOSVersionFrom prefers the os-release distribution name and falls back to
// the platform command (the kernel release on Linux) when it is absent or
// rejected.
func readOSVersionFrom(path string, fallback func() string) string {
	if v := osReleaseVersion(path); v != "" {
		return v
	}
	return fallback()
}

// osReleaseVersion returns a distribution name such as "Ubuntu 24.04" from an
// os-release file: PRETTY_NAME when present, else NAME plus VERSION_ID. The
// read is bounded and the value sanitised; "" means fall back.
func osReleaseVersion(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 16<<10))
	if err != nil {
		return ""
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || (key != "PRETTY_NAME" && key != "NAME" && key != "VERSION_ID") {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		fields[key] = value
	}
	value := fields["PRETTY_NAME"]
	if value == "" && fields["NAME"] != "" {
		value = strings.TrimSpace(fields["NAME"] + " " + fields["VERSION_ID"])
	}
	return sanitiseOSVersion(value)
}

func commandOSVersion() string {
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
	return sanitiseOSVersion(string(out))
}

// sanitiseOSVersion applies the same rule Capabilities.Validate enforces, so a
// value that passes here can never make the connector's own hello invalid.
func sanitiseOSVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !terminalproto.ValidPlainText(value, 128) {
		return ""
	}
	return value
}
