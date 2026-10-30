package terminalproto

import "strings"

// ShellDirFeature lets a controller start a plain shell in a chosen folder.
// A connector without it never receives a direction for a plain shell.
const ShellDirFeature = "shell-dir-v1"

// SupportsShellDir reports whether the peer advertised shell-dir-v1.
func SupportsShellDir(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ShellDirFeature {
			return true
		}
	}
	return false
}
