package web

import (
	"encoding/xml"
	"net/http"
	"strings"
	"testing"
)

// TestSitemapURLsArePublicAndSelfCanonical is the crawl contract: every address the sitemap advertises must be served,
// indexable and canonical to itself. It catches a public page that quietly becomes a 404, a noindex or a duplicate of
// the homepage.
func TestSitemapURLsArePublicAndSelfCanonical(t *testing.T) {
	requireBuilt(t)
	h := Handler("https://reet.sh",
		WithPublicPage(func(path string) (PublicPage, bool) {
			switch path {
			case "/community":
				return PublicPage{Path: "/community", Title: "Public community · reet.sh", Description: "Published lists and their skills.", HTML: `<main class="crawl-fallback"><h1>Public community</h1><p>Published lists and their skills.</p></main>`}, true
			case "/l/demo":
				return PublicPage{Path: "/l/demo", Title: "Demo list · reet.sh", Description: "A public list.", HTML: `<main class="crawl-fallback"><h1>Demo list</h1><p>A public list.</p></main>`}, true
			}
			return PublicPage{}, false
		}),
		WithSitemapURLs(func() []string { return []string{"/community", "/l/demo"} }),
	)
	rec := get(t, h, "/sitemap.xml")
	if rec.Code != http.StatusOK {
		t.Fatalf("sitemap: status = %d", rec.Code)
	}
	var doc sitemap
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid sitemap XML: %v", err)
	}
	if len(doc.URLs) < 4 {
		t.Fatalf("sitemap has only %d urls", len(doc.URLs))
	}
	for _, entry := range doc.URLs {
		loc := entry.Loc
		if !strings.HasPrefix(loc, "https://reet.sh/") {
			t.Errorf("unexpected origin in sitemap: %s", loc)
			continue
		}
		path := strings.TrimPrefix(loc, "https://reet.sh")
		page := get(t, h, path)
		if page.Code != http.StatusOK {
			t.Errorf("%s: sitemap URL returned %d", path, page.Code)
			continue
		}
		if strings.Contains(page.Header().Get("X-Robots-Tag"), "noindex") || strings.Contains(page.Body.String(), `name="robots" content="noindex`) {
			t.Errorf("%s: sitemap URL is noindex", path)
		}
		if path != "/" {
			want := `rel="canonical" href="` + loc + `"`
			if !strings.Contains(page.Body.String(), want) {
				t.Errorf("%s: canonical is not self-referential", path)
			}
		}
	}
	for _, private := range []string{"/signin", "/admin/overview", "/account", "/lists/1", "/sessions"} {
		if strings.Contains(rec.Body.String(), "https://reet.sh"+private) {
			t.Errorf("sitemap lists private route %s", private)
		}
	}
}
