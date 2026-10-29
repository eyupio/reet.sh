package terminalproto

import "strings"

// ClipboardFeature lets a machine forward an agent's OSC 52 clipboard write to
// the attached browser. The browser still decides, from a user gesture,
// whether to accept the write.
const ClipboardFeature = "clipboard-v1"

// SupportsClipboard reports whether the peer advertised clipboard-v1 in
// X-Reet-Terminal-Features. Capabilities.Clipboard is never set without it.
func SupportsClipboard(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ClipboardFeature {
			return true
		}
	}
	return false
}
