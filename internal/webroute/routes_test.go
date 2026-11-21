package webroute

import "testing"

func TestAppPagesArePages(t *testing.T) {
	for _, path := range []string{
		"/", "/factory", "/prompts", "/prompts/", "/usage", "/usage/", "/desks", "/desks/",
		"/desks/0123456789abcdef0123456789abcdef", "/desks/0123456789abcdef0123456789abcdef/",
		"/sync/advanced", "/sync/advanced/", "/account/mcp/authorize", "/lists/1", "/l/example", "/support/12",
		"/skill-insights", "/skill-insights/", "/admin/overview", "/join/token",
	} {
		if !Page(path) {
			t.Errorf("%s should be served as an app page", path)
		}
	}
}

func TestUnknownAndHostedPathsAreNotPages(t *testing.T) {
	for _, path := range []string{
		"/does-not-exist-xyz", "/desks/a/b", "/desks//", "/machines/demo", "/alice/reet-demo",
		"/sync/other", "/sync/advanced/more", "/assets/missing.js",
	} {
		if Page(path) {
			t.Errorf("%s is not an app page", path)
		}
	}
}
