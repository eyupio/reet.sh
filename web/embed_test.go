package web

import (
	"encoding/xml"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// requireBuilt skips only when the SPA has not been built, which is the case in
// Go-only CI jobs. The release pipeline builds web/ before running these tests.
func requireBuilt(t *testing.T) {
	t.Helper()
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		t.Skip("embedded dist is missing")
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		t.Skip("web/dist is not built; run `npm run build` in web/")
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRobotsTxtIsNotTheSPA(t *testing.T) {
	rec := get(t, Handler("https://reet.sh"), "/robots.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", ct)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Fatal("robots.txt served the SPA instead of a text file")
	}
	if !strings.Contains(body, "Sitemap: https://reet.sh/sitemap.xml") {
		t.Fatalf("robots.txt missing sitemap directive:\n%s", body)
	}
	for _, bot := range []string{"OAI-SearchBot", "Claude-SearchBot", "PerplexityBot", "Googlebot"} {
		if !strings.Contains(body, "User-agent: "+bot) {
			t.Errorf("robots.txt missing %s", bot)
		}
	}
}

type sitemap struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

func TestSitemapIsValidXML(t *testing.T) {
	rec := get(t, Handler("https://reet.sh"), "/sitemap.xml")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Fatalf("content-type = %q, want application/xml", ct)
	}
	var doc sitemap
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid sitemap XML: %v", err)
	}
	if len(doc.URLs) != 1 || doc.URLs[0].Loc != "https://reet.sh/" {
		t.Fatalf("unexpected sitemap urls: %+v", doc.URLs)
	}
}

func TestCrawlerFilesDeriveOriginFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	req.Host = "reet.example:8443"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	Handler("").ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Sitemap: https://reet.example:8443/sitemap.xml") {
		t.Fatalf("origin not derived from request:\n%s", rec.Body.String())
	}
}

func TestIndexHasCrawlableFallback(t *testing.T) {
	requireBuilt(t)
	rec := get(t, Handler("https://reet.sh"), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, originToken) {
		t.Fatal("origin token was not substituted")
	}
	if !strings.Contains(body, `rel="canonical" href="https://reet.sh/"`) {
		t.Error("missing canonical")
	}
	if !strings.Contains(body, `"@type": "SoftwareApplication"`) {
		t.Error("missing SoftwareApplication JSON-LD")
	}
	if !strings.Contains(body, "self-hosted workspace") {
		t.Error("missing crawlable landing copy")
	}
	if strings.Contains(body, "<!--seo-fallback:start-->") {
		t.Error("fallback markers should be removed from the served page")
	}
}

func TestWelcomeKeepsFallbackAndAppRoutesDoNot(t *testing.T) {
	requireBuilt(t)
	h := Handler("https://reet.sh")
	if body := get(t, h, "/welcome").Body.String(); !strings.Contains(body, "self-hosted workspace") {
		t.Error("/welcome should keep the crawlable landing copy")
	}
	if body := get(t, h, "/signin").Body.String(); strings.Contains(body, "self-hosted workspace") {
		t.Error("/signin should not serve the marketing fallback")
	}
}

func TestStaticAssetsStillServed(t *testing.T) {
	requireBuilt(t)
	rec := get(t, Handler(""), "/favicon.svg")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestOriginRejectsHeaderInjection(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	req.Host = "reet.sh"
	req.Header.Set("X-Forwarded-Proto", `https"><script>alert(1)</script>`)
	rec := httptest.NewRecorder()
	Handler("").ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, "<script>") || strings.Contains(body, `"><`) {
		t.Fatalf("header value was reflected: %s", body)
	}
	if !strings.Contains(body, "Sitemap: http://reet.sh/sitemap.xml") {
		t.Fatalf("invalid protocol should fall back to http:\n%s", body)
	}
}
