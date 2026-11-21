// Package webroute holds the addresses the web interface draws, so the SPA
// fallback and the public-skill router agree on what is an app page rather than
// a hosted skill. Keeping one list here means a new page cannot be captured by
// the two-segment skill route before the SPA ever sees it.
package webroute

import "strings"

// exact are addresses the SPA draws itself.
var exact = []string{
	"/", "/welcome", "/signin", "/signup", "/setup", "/reset", "/join",
	"/sessions", "/machines", "/skills", "/library", "/activity", "/sync", "/sync/advanced", "/sync/advanced/", "/hosting",
	"/community", "/shared", "/spaces", "/profiles", "/setups", "/prompts", "/prompts/", "/usage", "/usage/",
	"/notifications", "/support", "/factory", "/desks", "/desks/",
	"/admin", "/account",
}

// prefixes are page families whose address continues past the first segment.
var prefixes = []string{
	"/admin/", "/account/", "/join/", "/lists/", "/l/", "/support/",
}

// Page reports whether path is an address the SPA knows how to draw. Anything
// else is a real 404 (or a hosted skill), never a soft HTML page.
func Page(path string) bool {
	for _, p := range exact {
		if path == p {
			return true
		}
	}
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	// A desk is addressed by exactly one id; a deeper path is not a page.
	if id, ok := strings.CutPrefix(path, "/desks/"); ok {
		id = strings.TrimSuffix(id, "/")
		return id != "" && !strings.Contains(id, "/")
	}
	return false
}
