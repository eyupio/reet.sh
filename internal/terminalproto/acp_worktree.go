package terminalproto

import "strings"

// ACPWorktreeFeature distinguishes structured starts from the older terminal-
// only worktree contract. An older connector must never receive the new pair.
const ACPWorktreeFeature = "acp-worktree-v1"

func SupportsACPWorktree(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ACPWorktreeFeature {
			return true
		}
	}
	return false
}
