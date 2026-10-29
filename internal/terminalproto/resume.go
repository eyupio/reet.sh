package terminalproto

import "strings"

const ResumeFeature = "resume-v1"

// SupportsResume reports whether the peer advertised resume-v1 in
// X-Reet-Terminal-Features. A resume field is never sent without it.
func SupportsResume(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ResumeFeature {
			return true
		}
	}
	return false
}
