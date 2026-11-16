package terminalproto

import "strings"

// DeskShellFeature lets a machine report each running session's live working
// folder (SessionSnapshot.Dir, SessionChanged.Dir). A connection without it
// never carries the field: an older controller's strict decoder would refuse
// it, and an older machine simply never sends it.
const DeskShellFeature = "desk-shell-v1"

// SupportsDeskShell reports whether the peer advertised desk-shell-v1.
func SupportsDeskShell(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == DeskShellFeature {
			return true
		}
	}
	return false
}

// validLiveDir accepts an absent folder or an absolute, plain one.
func validLiveDir(dir string) bool { return dir == "" || validDir(dir) }
