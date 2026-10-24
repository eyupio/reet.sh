package scan

import "strings"

// buildCommit is embedded by the release workflow. It travels with every agent
// request so the controller can distinguish dev builds from one another.
var buildCommit = "unknown"

// SetBuildCommit records the source commit for this process. Call it once from
// main before any agent or scan work starts.
func SetBuildCommit(commit string) {
	if value := strings.TrimSpace(commit); value != "" {
		buildCommit = value
	}
}
