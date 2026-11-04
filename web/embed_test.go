package web

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"io"
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
		// A named crawler ignores the * group, so the private rules must be repeated in its own group.
		if !strings.Contains(body, "User-agent: "+bot+"\nAllow: /\nDisallow: /api/") {
			t.Errorf("robots.txt gives %s no private-path rules", bot)
		}
	}
	for _, path := range []string{"/admin", "/account", "/lists/", "/sessions", "/machines", "/skills", "/signin", "/setup"} {
		if !strings.Contains(body, "Disallow: "+path+"\n") {
			t.Errorf("robots.txt does not disallow %s", path)
		}
	}
}

func TestCrawlerTextFilesArePlainText(t *testing.T) {
	h := Handler("https://reet.sh")
	for _, path := range []string{"/llms.txt", "/llms-full.txt", "/ai.txt"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Fatalf("%s: content-type = %q, want text/plain", path, ct)
		}
		body := rec.Body.String()
		if strings.Contains(body, "<html") || strings.Contains(body, "data-boot") {
			t.Fatalf("%s served the SPA instead of text:\n%s", path, body)
		}
		if !strings.Contains(body, "reet.sh") {
			t.Fatalf("%s does not name the product", path)
		}
	}
	missing := get(t, h, "/missing.txt")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing text file: status = %d, want 404", missing.Code)
	}
}

func TestUnknownRouteIsAReal404(t *testing.T) {
	requireBuilt(t)
	h := Handler("https://reet.sh")
	for _, path := range []string{"/does-not-exist-xyz", "/docs/mcp", "/assets/missing.js", "/sitemap-index.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
		if rec.Code == http.StatusNotFound && !strings.Contains(rec.Header().Get("X-Robots-Tag"), "noindex") {
			t.Errorf("%s: missing noindex on a 404", path)
		}
	}
}

// The app screens are not search results. Public pages must stay indexable.
func TestPrivateRoutesAreNoindexAndPublicRoutesAreNot(t *testing.T) {
	requireBuilt(t)
	h := Handler("https://reet.sh")
	private := []string{"/signin", "/signup", "/setup", "/reset", "/join?token=t", "/admin/overview", "/account", "/lists/1", "/sessions", "/machines", "/skills", "/activity", "/sync", "/hosting", "/notifications", "/support", "/shared", "/spaces", "/profiles", "/setups"}
	for _, path := range private {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d", path, rec.Code)
		}
		if !strings.Contains(rec.Header().Get("X-Robots-Tag"), "noindex") {
			t.Errorf("%s: missing X-Robots-Tag noindex", path)
		}
		if !strings.Contains(rec.Body.String(), `name="robots" content="noindex`) {
			t.Errorf("%s: missing meta robots noindex", path)
		}
	}
	for _, path := range []string{"/", "/welcome", "/community", "/l/example"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d", path, rec.Code)
		}
		if strings.Contains(rec.Header().Get("X-Robots-Tag"), "noindex") || strings.Contains(rec.Body.String(), `name="robots" content="noindex`) {
			t.Errorf("%s: public page is marked noindex", path)
		}
	}
}

func TestLegacyRoutesRedirectPermanently(t *testing.T) {
	h := Handler("https://reet.sh")
	cases := map[string]string{
		"/control":       "/",
		"/terminals":     "/sessions",
		"/terminals?x=1": "/sessions?x=1",
		"/sessions/":     "/sessions",
		"/projects":      "/",
		"/projects/abc":  "/?project=abc",
	}
	for from, want := range cases {
		rec := get(t, h, from)
		if rec.Code != http.StatusMovedPermanently {
			t.Errorf("%s: status = %d, want 301", from, rec.Code)
			continue
		}
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("%s: Location = %q, want %q", from, got, want)
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
	if !strings.Contains(body, "control room for your AI agents") {
		t.Error("missing crawlable landing copy")
	}
	if strings.Contains(body, "<!--seo-fallback:start-->") {
		t.Error("fallback markers should be removed from the served page")
	}
}

func TestWelcomeKeepsFallbackAndAppRoutesDoNot(t *testing.T) {
	requireBuilt(t)
	h := Handler("https://reet.sh")
	if body := get(t, h, "/welcome").Body.String(); !strings.Contains(body, "control room for your AI agents") {
		t.Error("/welcome should keep the crawlable landing copy")
	}
	if body := get(t, h, "/signin").Body.String(); strings.Contains(body, "control room for your AI agents") {
		t.Error("/signin should not serve the marketing fallback")
	}
}

func TestSignedInLandingRequestHidesFallback(t *testing.T) {
	requireBuilt(t)
	signedIn := Handler("https://reet.sh", WithSignedIn(func(*http.Request) bool { return true }))
	if body := get(t, signedIn, "/").Body.String(); strings.Contains(body, "control room for your AI agents") {
		t.Error("a signed-in landing request should not serve the marketing fallback")
	}
	anonymous := Handler("https://reet.sh", WithSignedIn(func(*http.Request) bool { return false }))
	if body := get(t, anonymous, "/").Body.String(); !strings.Contains(body, "control room for your AI agents") {
		t.Error("an anonymous landing request should keep the crawlable landing copy")
	}
}

// The boot shell must match the route: a landing hero for an anonymous "/", a centred card for the entry screens, and the
// app's page shape everywhere else. Serving the wrong one is the half-drawn screen this exists to remove.
func TestBootShellMatchesTheRoute(t *testing.T) {
	requireBuilt(t)
	anonymous := Handler("https://reet.sh")
	signedIn := Handler("https://reet.sh", WithSignedIn(func(*http.Request) bool { return true }))
	cases := []struct {
		name string
		h    http.Handler
		path string
		want string
	}{
		{"anonymous landing", anonymous, "/", `data-boot="welcome"`},
		{"welcome", anonymous, "/welcome", `data-boot="welcome"`},
		{"sign in", anonymous, "/signin", `data-boot="auth"`},
		{"sign up", anonymous, "/signup", `data-boot="auth"`},
		{"join", anonymous, "/join?token=t", `data-boot="auth"`},
		{"app page", anonymous, "/machines", `data-boot="app"`},
		{"signed-in landing is Home", signedIn, "/", `data-boot="app"`},
	}
	for _, c := range cases {
		body := get(t, c.h, c.path).Body.String()
		if !strings.Contains(body, c.want) {
			t.Errorf("%s: missing %q", c.name, c.want)
		}
		// The document itself names the shell, so the browser does not have to guess (and cannot see a cookie).
		if !strings.Contains(body, `<html lang="en" `+c.want+`>`) {
			t.Errorf("%s: <html> does not name %q", c.name, c.want)
		}
		for _, other := range []string{`data-boot="welcome"`, `data-boot="auth"`, `data-boot="app"`} {
			if other != c.want && strings.Contains(body, other) {
				t.Errorf("%s: served %q as well as %q", c.name, other, c.want)
			}
		}
		if strings.Contains(body, "<!--boot-") {
			t.Errorf("%s: boot markers were left in the served page", c.name)
		}
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

// The registration itself, not mime.TypeByExtension: a host with /etc/mime.types already maps .woff2, so the served header
// alone would pass without it.
func TestPackageRegistersTypesTheHostMayLack(t *testing.T) {
	got := map[string]string{}
	registerTypes(func(ext, typ string) error { got[ext] = typ; return nil })
	for ext, want := range map[string]string{".woff2": "font/woff2", ".webmanifest": "application/manifest+json"} {
		if got[ext] != want {
			t.Errorf("%s registered as %q, want %q", ext, got[ext], want)
		}
	}
}

func TestSelfHostedFontsServedAsWOFF2(t *testing.T) {
	requireBuilt(t)
	for _, name := range []string{"fraunces-latin-600-normal", "ibm-plex-sans-latin-400-normal", "ibm-plex-sans-latin-500-normal", "ibm-plex-sans-latin-600-normal", "ibm-plex-mono-latin-400-normal"} {
		rec := get(t, Handler(""), "/fonts/"+name+".woff2")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", name, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "font/woff2" {
			t.Fatalf("%s: content-type = %q", name, ct)
		}
	}
	if rec := get(t, Handler(""), "/fonts/OFL.txt"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SIL OPEN FONT LICENSE") {
		t.Fatalf("OFL.txt: status = %d", rec.Code)
	}
}

// builtAsset finds one hashed file of the given extension under dist/assets, which only exists once the UI is built.
func builtAsset(t *testing.T, ext string) string {
	t.Helper()
	requireBuilt(t)
	sub, _ := fs.Sub(dist, "dist")
	entries, err := fs.ReadDir(sub, "assets")
	if err != nil {
		t.Skip("web/dist has no assets directory")
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ext) {
			return "/assets/" + e.Name()
		}
	}
	t.Skipf("web/dist/assets has no %s file", ext)
	return ""
}

// The bundle is over a megabyte of JavaScript and CSS, and a browser on a slow link waited for all of it before the
// boot shell could be painted. Compressed once and cached for a year under its content hash, it is a few hundred
// kilobytes the first time and nothing at all afterwards.
func TestHashedAssetsAreCompressedAndCachedForAYear(t *testing.T) {
	for _, ext := range []string{".js", ".css"} {
		p := builtAsset(t, ext)
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		rec := httptest.NewRecorder()
		Handler("").ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", p, rec.Code)
		}
		if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
			t.Errorf("%s: content-encoding = %q, want gzip", p, enc)
		}
		if vary := rec.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
			t.Errorf("%s: vary = %q, want Accept-Encoding", p, vary)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
			t.Errorf("%s: cache-control = %q", p, cc)
		}
		if cl := rec.Header().Get("Content-Length"); cl == "" || cl == "0" {
			t.Errorf("%s: a compressed body needs its length for the browser's progress bar, got %q", p, cl)
		}
		zr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatalf("%s: body is not gzip: %v", p, err)
		}
		plain, err := io.ReadAll(zr)
		if err != nil {
			t.Fatalf("%s: reading the gzip body: %v", p, err)
		}
		sub, _ := fs.Sub(dist, "dist")
		want, _ := fs.ReadFile(sub, strings.TrimPrefix(p, "/"))
		if !bytes.Equal(plain, want) {
			t.Errorf("%s: the compressed body does not decompress to the file (%d bytes, want %d)", p, len(plain), len(want))
		}
		if len(rec.Body.Bytes()) >= len(want) {
			t.Errorf("%s: compressed body (%d bytes) is no smaller than the file (%d)", p, rec.Body.Len(), len(want))
		}
	}
}

// A client that does not take gzip, or one that refuses it with q=0, gets the file as it is, with the same caching.
func TestAssetsAreServedPlainToAClientThatDoesNotTakeGzip(t *testing.T) {
	p := builtAsset(t, ".js")
	for _, accept := range []string{"", "gzip;q=0, identity", "br"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		if accept != "" {
			req.Header.Set("Accept-Encoding", accept)
		}
		rec := httptest.NewRecorder()
		Handler("").ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("accept %q: status = %d", accept, rec.Code)
		}
		if enc := rec.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("accept %q: content-encoding = %q, want none", accept, enc)
		}
		if vary := rec.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
			t.Errorf("accept %q: vary = %q; a cache must keep the plain and gzip bodies apart", accept, vary)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
			t.Errorf("accept %q: cache-control = %q", accept, cc)
		}
	}
}

// Fonts are already compressed and are not content-hashed: gzip would make them larger and a year's cache would
// outlive a changed file, so they get neither.
func TestFontsAreNeitherCompressedNorCachedForAYear(t *testing.T) {
	requireBuilt(t)
	req := httptest.NewRequest(http.MethodGet, "/fonts/fraunces-latin-600-normal.woff2", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	Handler("").ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("content-encoding = %q, want none", enc)
	}
	if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("cache-control = %q; an unhashed file must not be immutable", cc)
	}
}

// index.html carries the request's origin and the boot shell for its route, so it is made per request; it is still
// twenty kilobytes of markup and inline styles that a slow link is better off receiving compressed.
func TestIndexIsCompressedWhenTheClientTakesGzip(t *testing.T) {
	requireBuilt(t)
	req := httptest.NewRequest(http.MethodGet, "/machines", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	Handler("").ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("content-encoding = %q, want gzip", enc)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("cache-control = %q; the page itself must stay no-cache", cc)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	plain, _ := io.ReadAll(zr)
	if !strings.Contains(string(plain), `data-boot="app"`) {
		t.Errorf("the decompressed page is not the app shell for /machines")
	}
}

func TestPublicPagesAreServerRenderedWithTheirOwnMetadata(t *testing.T) {
	requireBuilt(t)
	h := Handler("https://reet.sh", WithPublicPage(func(path string) (PublicPage, bool) {
		switch path {
		case "/community":
			return PublicPage{Path: "/community", Title: "Public community · reet.sh", Description: "Published lists and their skills.", HTML: `<main class="crawl-fallback"><h1>Public community</h1><p>Browse published lists and their skills.</p></main>`}, true
		case "/l/demo":
			return PublicPage{Path: "/l/demo", Title: "Demo list · reet.sh", Description: "A public list of review skills.", HTML: `<main class="crawl-fallback"><h1>Demo list</h1><p>Code review and release skills.</p></main>`}, true
		}
		return PublicPage{}, false
	}))
	for _, c := range []struct {
		path, title, description, html string
	}{
		{"/community", "Public community · reet.sh", "Published lists and their skills.", "Browse published lists"},
		{"/l/demo", "Demo list · reet.sh", "A public list of review skills.", "Code review and release skills."},
	} {
		rec := get(t, h, c.path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", c.path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "<title>"+c.title+"</title>") {
			t.Errorf("%s: missing title %q", c.path, c.title)
		}
		if !strings.Contains(body, `content="`+c.description+`"`) {
			t.Errorf("%s: missing description", c.path)
		}
		if !strings.Contains(body, `rel="canonical" href="https://reet.sh`+c.path+`"`) {
			t.Errorf("%s: canonical is not self-referential", c.path)
		}
		if !strings.Contains(body, `property="og:url" content="https://reet.sh`+c.path+`"`) {
			t.Errorf("%s: og:url is not self-referential", c.path)
		}
		if !strings.Contains(body, c.html) {
			t.Errorf("%s: missing server-rendered content", c.path)
		}
		if strings.Contains(body, "One command on any machine") {
			t.Errorf("%s: still served the landing fallback", c.path)
		}
	}
}

func TestPublicPageCanReturnA404(t *testing.T) {
	requireBuilt(t)
	h := Handler("https://reet.sh", WithPublicPage(func(path string) (PublicPage, bool) {
		return PublicPage{Path: path, Title: "That link is not available · reet.sh", Description: "The list is private or gone.", HTML: `<main class="crawl-fallback"><h1>That link is not available.</h1></main>`, Status: http.StatusNotFound}, true
	}))
	rec := get(t, h, "/l/gone")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("X-Robots-Tag"), "noindex") {
		t.Fatal("missing noindex on a missing public page")
	}
}

func TestSitemapIncludesSuppliedPublicPaths(t *testing.T) {
	h := Handler("https://reet.sh", WithSitemapURLs(func() []string {
		return []string{"/community", "/l/demo", "/community", "/", "/l/demo"}
	}))
	rec := get(t, h, "/sitemap.xml")
	var doc sitemap
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid sitemap XML: %v", err)
	}
	got := make([]string, 0, len(doc.URLs))
	for _, u := range doc.URLs {
		got = append(got, u.Loc)
	}
	want := []string{"https://reet.sh/", "https://reet.sh/community", "https://reet.sh/l/demo"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sitemap urls = %v, want %v", got, want)
	}
}
