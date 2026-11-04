package docs

import (
	"strings"
	"testing"
)

func TestPagesAreRenderedAndDiscoverable(t *testing.T) {
	pages := Pages()
	if len(pages) < 20 {
		t.Fatalf("only %d pages", len(pages))
	}
	seen := map[string]bool{}
	for _, page := range pages {
		if page.Slug == "" || page.Title == "" || page.Description == "" || page.HTML == "" {
			t.Errorf("incomplete page: %+v", page)
		}
		if seen[page.Slug] {
			t.Errorf("duplicate slug %q", page.Slug)
		}
		seen[page.Slug] = true
		if len(page.Description) > 155 {
			t.Errorf("%s description is %d characters", page.Slug, len(page.Description))
		}
	}
	for _, slug := range []string{"binary-installation", "mcp", "controller-sync", "backups", "cli-reference"} {
		if _, ok := Get(slug); !ok {
			t.Errorf("missing page %q", slug)
		}
	}
}

func TestRelativeLinksAndImagesPointAtTheDocsSite(t *testing.T) {
	mcp, ok := Get("mcp")
	if !ok {
		t.Fatal("missing mcp page")
	}
	if !strings.Contains(strings.ToLower(mcp.HTML), "<h1") {
		t.Fatal("missing rendered heading")
	}
	// Every relative Markdown link is rewritten to a docs URL or the repository, never left as a .md file.
	if strings.Contains(mcp.HTML, `href="`) {
		for _, part := range strings.Split(mcp.HTML, `href="`) {
			target, _, _ := strings.Cut(part, `"`)
			if strings.HasSuffix(target, ".md") && !strings.HasPrefix(target, "http") {
				t.Errorf("unrewritten markdown link %q", target)
			}
		}
	}
}
