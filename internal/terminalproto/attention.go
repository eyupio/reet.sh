package terminalproto

import "strings"

// AttentionFeature negotiates terminal activity, bell and exit-status metadata
// on session snapshots and state changes. No terminal output is ever carried.
const AttentionFeature = "attention-v1"

// SupportsAttention reports whether features includes attention-v1.
func SupportsAttention(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == AttentionFeature {
			return true
		}
	}
	return false
}

func validAttentionTime(n int64) bool {
	return n >= 0 && n <= MaxCounter
}

// ForFeatures strips fields the peer's negotiated feature header does not
// allow, so an older controller's strict decoder never sees unknown fields.
func (s SessionSnapshot) ForFeatures(features string) SessionSnapshot {
	if !SupportsAttention(features) {
		s.ActivityAt = 0
		s.BellAt = 0
		s.ExitStatus = nil
	}
	if !SupportsACPHistory(features) {
		s.ACPSessionID = ""
	}
	return s
}

// ForFeatures strips attention fields when attention-v1 was not negotiated.
func (s SessionChanged) ForFeatures(features string) SessionChanged {
	if !SupportsAttention(features) {
		s.ActivityAt = 0
		s.BellAt = 0
		s.ExitStatus = nil
	}
	if !SupportsACPHistory(features) {
		s.ACPSessionID = ""
	}
	return s
}
