// Package web embeds the built Svelte SPA.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// extraTypes are the types the served files need that Go's built-in table lacks. Without them a type comes from the host's
// mime.types, which a slim image does not have: the web app manifest would be text/plain and the self-hosted fonts untyped.
var extraTypes = [][2]string{
	{".webmanifest", "application/manifest+json"},
	{".woff2", "font/woff2"},
}

// registerTypes hands each extra type to add (mime.AddExtensionType when the package loads), so a test can see the
// registration itself rather than whatever the host's mime.types happens to say.
func registerTypes(add func(ext, typ string) error) {
	for _, t := range extraTypes {
		_ = add(t[0], t[1])
	}
}

func init() { registerTypes(mime.AddExtensionType) }

//go:embed all:dist
var dist embed.FS

const (
	originToken   = "{{ORIGIN}}"
	fallbackStart = "<!--seo-fallback:start-->"
	fallbackEnd   = "<!--seo-fallback:end-->"
)

// The boot shell in index.html holds the page's shape while the bundle loads. Only the variant that matches the route is
// served, so a landing request never holds an app skeleton and an app request never holds a landing hero.
var bootZones = [][2]string{
	{"<!--boot-welcome:start-->", "<!--boot-welcome:end-->"},
	{"<!--boot-app:start-->", "<!--boot-app:end-->"},
	{"<!--boot-auth:start-->", "<!--boot-auth:end-->"},
}

const (
	bootWelcome = iota
	bootApp
	bootAuth
)

var bootNames = [...]string{"welcome", "app", "auth"}

// bootVariant picks the shell that matches what the app is about to draw: the landing hero for an anonymous landing
// route, a centred card for the entry screens, and the page shell for everything else. A signed-in request to "/" is
// Home, not the landing page.
func bootVariant(path string, signedIn bool) int {
	if !signedIn && (path == "/" || path == "/welcome") {
		return bootWelcome
	}
	switch {
	case path == "/signin", path == "/signup", path == "/setup", path == "/reset", strings.HasPrefix(path, "/join"):
		return bootAuth
	}
	return bootApp
}

// Option configures Handler.
type Option func(*config)

type config struct {
	signedIn   func(*http.Request) bool
	publicPage func(string) (PublicPage, bool)
	sitemapURL func() []string
}

// PublicPage is server-rendered content for a public SPA route. A crawler that does not run JavaScript gets the heading,
// description and links instead of an empty app shell. HTML must already be escaped.
type PublicPage struct {
	Path        string
	Title       string
	Description string
	HTML        string
	Status      int
}

// WithSignedIn tells Handler how to recognise a request that already carries a
// signed-in session. Such a visitor is shown the app shell, not the static
// landing page, so serving them the crawler fallback would flash the logged-out
// page before the SPA mounts.
func WithSignedIn(fn func(*http.Request) bool) Option {
	return func(c *config) { c.signedIn = fn }
}

// WithPublicPage supplies crawlable content and metadata for public routes such as /community and /l/<slug>.
func WithPublicPage(fn func(string) (PublicPage, bool)) Option {
	return func(c *config) { c.publicPage = fn }
}

// WithSitemapURLs supplies extra public paths for sitemap.xml. Each path must start with "/".
func WithSitemapURLs(fn func() []string) Option {
	return func(c *config) { c.sitemapURL = fn }
}

// Handler serves the SPA, falling back to index.html for client-side routes.
// The crawler files are answered here so the SPA fallback cannot shadow them
// with an HTML page, and index.html is served with the request origin applied.
// baseURL is the configured public origin (REET_PUBLIC_URL); when empty the
// origin is derived from the request.
func Handler(baseURL string, opts ...Option) http.Handler {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("web: embedded dist is missing: " + err.Error())
	}
	files := http.FileServerFS(sub)
	index, _ := fs.ReadFile(sub, "index.html")
	gz := &gzipCache{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveDocs(w, r, origin(r, baseURL)) {
			return
		}
		if target, ok := legacyRedirect(r); ok {
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		switch r.URL.Path {
		case "/robots.txt":
			writeRobots(w, origin(r, baseURL))
			return
		case "/sitemap.xml":
			writeSitemap(w, origin(r, baseURL), sitemapPaths(cfg.sitemapURL))
			return
		case "/llms.txt":
			writeText(w, r, llmsTxt, origin(r, baseURL))
			return
		case "/llms-full.txt":
			writeText(w, r, llmsFullTxt, origin(r, baseURL))
			return
		case "/ai.txt":
			writeText(w, r, aiTxt, origin(r, baseURL))
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" || p == "index.html" {
			serveIndex(w, r, index, r.URL.Path, origin(r, baseURL), cfg.signedIn != nil && cfg.signedIn(r), cfg.publicPage)
			return
		}
		if info, err := fs.Stat(sub, p); err == nil && !info.IsDir() {
			serveFile(w, r, sub, files, gz, p, info.Size())
			return
		}
		if index == nil {
			http.Error(w, "reet.sh UI is not built; run `npm run build` in web/", http.StatusNotFound)
			return
		}
		if !spaRoute(r.URL.Path) {
			serveIndexStatus(w, r, index, r.URL.Path, origin(r, baseURL), cfg.signedIn != nil && cfg.signedIn(r), http.StatusNotFound, cfg.publicPage)
			return
		}
		serveIndex(w, r, index, r.URL.Path, origin(r, baseURL), cfg.signedIn != nil && cfg.signedIn(r), cfg.publicPage)
	})
}

// legacyRedirect keeps old addresses working with a permanent redirect instead of letting the SPA rewrite them in the
// browser. A crawler that reaches them sees one canonical target and does not index a duplicate.
func legacyRedirect(r *http.Request) (string, bool) {
	path := r.URL.Path
	query := ""
	if r.URL.RawQuery != "" {
		query = "?" + r.URL.RawQuery
	}
	switch {
	case path == "/control" || path == "/control/":
		return "/" + query, true
	case path == "/terminals" || path == "/terminals/":
		return "/sessions" + query, true
	case path == "/sessions/":
		return "/sessions" + query, true
	case path == "/projects" || path == "/projects/":
		return "/" + query, true
	case strings.HasPrefix(path, "/projects/"):
		id := strings.TrimPrefix(path, "/projects/")
		if id == "" {
			return "/" + query, true
		}
		return "/?project=" + url.QueryEscape(id), true
	}
	return "", false
}

// spaRoute is the set of addresses the SPA knows how to draw. Anything else is a real 404: returning index.html with 200
// for an unknown path is a soft 404, and it makes missing text and XML files look like successful HTML responses.
func spaRoute(path string) bool {
	switch path {
	case "/", "/welcome", "/signin", "/signup", "/setup", "/reset", "/join",
		"/sessions", "/machines", "/skills", "/activity", "/sync", "/hosting",
		"/community", "/shared", "/spaces", "/profiles", "/setups", "/notifications", "/support",
		"/admin", "/account":
		return true
	}
	for _, prefix := range []string{
		"/admin/", "/account/", "/join/", "/lists/", "/l/", "/support/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// publicIndexRoute is the subset of SPA routes meant for search engines. Every other SPA route is an app screen behind
// sign-in, or an empty state, and is served with noindex.
func publicIndexRoute(path string) bool {
	switch path {
	case "/", "/welcome", "/community":
		return true
	}
	return strings.HasPrefix(path, "/l/")
}

// serveIndex serves the built index.html with the origin substituted. The
// static crawler fallback is only kept on landing routes for visitors who are
// not already signed in; every other route is an app screen, and a signed-in
// visitor is going straight to the app, so both would otherwise paint the
// logged-out landing page before the SPA mounts.
func serveIndex(w http.ResponseWriter, r *http.Request, index []byte, path, origin string, signedIn bool, public func(string) (PublicPage, bool)) {
	serveIndexStatus(w, r, index, path, origin, signedIn, http.StatusOK, public)
}

func serveIndexStatus(w http.ResponseWriter, r *http.Request, index []byte, path, origin string, signedIn bool, status int, public func(string) (PublicPage, bool)) {
	if index == nil {
		http.Error(w, "reet.sh UI is not built; run `npm run build` in web/", http.StatusNotFound)
		return
	}
	body := bytes.ReplaceAll(index, []byte(originToken), []byte(html.EscapeString(origin)))
	page, hasPage := PublicPage{}, false
	if public != nil {
		page, hasPage = public(path)
	}
	if hasPage && page.Status != 0 {
		status = page.Status
	}
	switch {
	case hasPage:
		body = replaceBetween(body, []byte(fallbackStart), []byte(fallbackEnd), []byte(page.HTML))
		body = bytes.ReplaceAll(body, []byte(fallbackStart), nil)
		body = bytes.ReplaceAll(body, []byte(fallbackEnd), nil)
		body = applyPublicHead(body, page, path, origin)
	case !signedIn && (path == "/" || path == "/welcome"):
		body = bytes.ReplaceAll(body, []byte(fallbackStart), nil)
		body = bytes.ReplaceAll(body, []byte(fallbackEnd), nil)
	default:
		body = stripFallback(body)
	}
	variant := bootVariant(path, signedIn)
	body = selectBoot(body, variant)
	// The chosen shell is on the document before the head script runs, so a cookie-signed-in visitor on "/" gets the app
	// shell rather than the landing hero the browser alone cannot know to skip.
	body = bytes.Replace(body, []byte(`<html lang="en">`), []byte(`<html lang="en" data-boot="`+bootNames[variant]+`">`), 1)
	if status != http.StatusOK || !publicIndexRoute(path) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		body = bytes.Replace(body, []byte("<head>"), []byte("<head>\n    <meta name=\"robots\" content=\"noindex, nofollow\" />"), 1)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	writeMaybeGzipStatus(w, r, body, status)
}

var (
	titleRE         = regexp.MustCompile(`(?s)<title>.*?</title>`)
	descriptionRE   = regexp.MustCompile(`(?s)<meta\s+name="description"[^>]*>`)
	canonicalRE     = regexp.MustCompile(`(?s)<link\s+rel="canonical"[^>]*>`)
	ogTitleRE       = regexp.MustCompile(`(?s)<meta\s+property="og:title"[^>]*>`)
	ogDescriptionRE = regexp.MustCompile(`(?s)<meta\s+property="og:description"[^>]*>`)
	ogURLRE         = regexp.MustCompile(`(?s)<meta\s+property="og:url"[^>]*>`)
	twitterTitleRE  = regexp.MustCompile(`(?s)<meta\s+name="twitter:title"[^>]*>`)
	twitterDescRE   = regexp.MustCompile(`(?s)<meta\s+name="twitter:description"[^>]*>`)
)

// applyPublicHead gives a server-rendered public page the same metadata a browser would see after the SPA mounts.
func applyPublicHead(body []byte, page PublicPage, path, origin string) []byte {
	if page.Title != "" {
		title := html.EscapeString(page.Title)
		body = replaceFirst(body, titleRE, "<title>"+title+"</title>")
		body = replaceFirst(body, ogTitleRE, `<meta property="og:title" content="`+title+`" />`)
		body = replaceFirst(body, twitterTitleRE, `<meta name="twitter:title" content="`+title+`" />`)
	}
	if page.Description != "" {
		description := html.EscapeString(page.Description)
		body = replaceFirst(body, descriptionRE, `<meta name="description" content="`+description+`" />`)
		body = replaceFirst(body, ogDescriptionRE, `<meta property="og:description" content="`+description+`" />`)
		body = replaceFirst(body, twitterDescRE, `<meta name="twitter:description" content="`+description+`" />`)
	}
	canonicalPath := page.Path
	if canonicalPath == "" {
		canonicalPath = path
	}
	canonical := html.EscapeString(strings.TrimRight(origin, "/") + canonicalPath)
	body = replaceFirst(body, canonicalRE, `<link rel="canonical" href="`+canonical+`" />`)
	body = replaceFirst(body, ogURLRE, `<meta property="og:url" content="`+canonical+`" />`)
	return body
}

func replaceFirst(body []byte, re *regexp.Regexp, replacement string) []byte {
	loc := re.FindIndex(body)
	if loc == nil {
		return body
	}
	out := make([]byte, 0, len(body)-(loc[1]-loc[0])+len(replacement))
	out = append(out, body[:loc[0]]...)
	out = append(out, replacement...)
	out = append(out, body[loc[1]:]...)
	return out
}

func replaceBetween(body []byte, start, end []byte, replacement []byte) []byte {
	i := bytes.Index(body, start)
	if i < 0 {
		return body
	}
	j := bytes.Index(body[i:], end)
	if j < 0 {
		return body
	}
	j += i
	out := make([]byte, 0, i+len(start)+len(replacement)+len(body)-(j))
	out = append(out, body[:i+len(start)]...)
	out = append(out, replacement...)
	out = append(out, body[j:]...)
	return out
}

// serveFile answers a built file: a content-hashed one is cacheable for a year, and a text one goes compressed to a
// client that takes gzip. Anything else (a font, an icon, a request for a byte range) is left to the file server, which
// knows about ranges and conditional requests; a range of a compressed body would not be a range of the file.
func serveFile(w http.ResponseWriter, r *http.Request, sub fs.FS, files http.Handler, gz *gzipCache, name string, size int64) {
	if hashedAsset(name) {
		w.Header().Set("Cache-Control", immutableCache)
	}
	if !compressibleAsset(name) || size < gzipMinSize {
		files.ServeHTTP(w, r)
		return
	}
	w.Header().Add("Vary", "Accept-Encoding")
	if r.Header.Get("Range") != "" || !acceptsGzip(r) {
		files.ServeHTTP(w, r)
		return
	}
	body, err := gz.body(name, func() ([]byte, error) { return fs.ReadFile(sub, name) })
	if err != nil {
		files.ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// The length is said because a browser shows its loading progress against it, and ServeContent leaves it off an
	// encoded body on the assumption that something downstream will change it. Nothing will.
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(body))
}

func stripFallback(body []byte) []byte {
	return stripBetween(body, []byte(fallbackStart), []byte(fallbackEnd))
}

// selectBoot keeps one boot shell and removes the other two, markers included, so the served page never carries a
// variant that does not match its route.
func selectBoot(body []byte, keep int) []byte {
	for i, zone := range bootZones {
		start, end := []byte(zone[0]), []byte(zone[1])
		if i == keep {
			body = bytes.ReplaceAll(body, start, nil)
			body = bytes.ReplaceAll(body, end, nil)
			continue
		}
		body = stripBetween(body, start, end)
	}
	return body
}

// stripBetween removes the range from start through end, markers included. A missing or reversed pair leaves the body
// unchanged, so an index built without a zone still serves.
func stripBetween(body, start, end []byte) []byte {
	i := bytes.Index(body, start)
	if i < 0 {
		return body
	}
	j := bytes.Index(body[i:], end)
	if j < 0 {
		return body
	}
	j += i + len(end)
	out := make([]byte, 0, len(body))
	out = append(out, body[:i]...)
	out = append(out, body[j:]...)
	return out
}

// origin is the absolute origin used in robots.txt, sitemap.xml and the
// origin-tagged tags in index.html.
func origin(r *http.Request, baseURL string) string {
	if base := strings.TrimRight(strings.TrimSpace(baseURL), "/"); base != "" {
		if u, err := url.Parse(base); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
			return u.Scheme + "://" + u.Host
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := r.Host
	if !validHost(host) {
		host = "localhost"
	}
	return scheme + "://" + host
}

// validHost keeps untrusted Host values out of the generated HTML and XML. Go's
// server already rejects malformed hosts, but proxies can pass arbitrary values.
func validHost(host string) bool {
	if host == "" || len(host) > 255 {
		return false
	}
	for _, c := range host {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.' || c == '-' || c == ':' || c == '[' || c == ']' || c == '_':
		default:
			return false
		}
	}
	return true
}

// robotsPrivate keeps the app, account and private-list screens out of search results. The public pages are /, /welcome,
// /community, /l/<slug>, hosted skill pages and /docs/. A crawler with its own group below ignores the * group, so the
// same rules are repeated for each named crawler.
const robotsPrivate = `Disallow: /api/
Disallow: /quick
Disallow: /signin
Disallow: /signup
Disallow: /setup
Disallow: /reset
Disallow: /join
Disallow: /admin
Disallow: /account
Disallow: /lists/
Disallow: /sessions
Disallow: /machines
Disallow: /skills
Disallow: /activity
Disallow: /sync
Disallow: /hosting
Disallow: /notifications
Disallow: /support
Disallow: /shared
Disallow: /spaces
Disallow: /profiles
Disallow: /setups
`

const robotsTxt = `# reet.sh — self-hosted control room for AI agents
# Search engines and AI search crawlers may index the public pages.
# Model-training crawlers are not restricted by default; add your own
# User-agent blocks if this instance should opt out of training.

User-agent: *
Allow: /
%[1]s
User-agent: OAI-SearchBot
Allow: /
%[1]s
User-agent: Claude-SearchBot
Allow: /
%[1]s
User-agent: PerplexityBot
Allow: /
%[1]s
User-agent: Googlebot
Allow: /
%[1]s
Sitemap: %[2]s/sitemap.xml
`

func writeRobots(w http.ResponseWriter, origin string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(fmt.Sprintf(robotsTxt, robotsPrivate, html.EscapeString(origin))))
}

// llmsTxt is a short index for assistants and agents. It points at public pages and the canonical source documentation
// until the docs site is served from this origin.
const llmsTxt = `# reet.sh

> A self-hosted control room for the AI agents, skills and setups across your machines.

## Public pages

- [reet.sh](%[1]s/): product overview and install command
- [Public community](%[1]s/community): published lists and their skills
- [Source and documentation](https://github.com/eyupio/reet.sh): README, guides and the AGPL-3.0 licence

## What it is

reet.sh runs as one Go binary with one SQLite file. It connects machines through an outbound agent, lets you open
terminals and start installed agents, and syncs skills and reusable setups with reviewable plans. It sends no telemetry.

## Notes

- The public community is readable without an account.
- App, account and private-list pages are not intended for indexing.
`

// llmsFullTxt is a compact factual summary for assistants that request the longer form.
const llmsFullTxt = `# reet.sh

reet.sh is a self-hosted control room for the AI agents, skills and reusable setups on your machines.

## Product

- Connect a laptop, VPS or lab machine with an outbound enrolment command; no inbound port is required.
- Open a browser terminal on any enrolled machine and work with agents already installed there.
- Start and continue sessions with Claude Code, Codex, OpenCode and Gemini CLI.
- Keep a central skill library, bring discoveries back, and distribute skills with a reviewed plan before anything changes.
- Save reusable setup profiles and restore them on a new machine.
- Back up the instance or a personal shelf with encrypted, verified copies.
- Run as one Go binary and one SQLite file. Accounts, token mode and no-auth mode are available.
- No telemetry is sent by default.

## Links

- Product: %[1]s/
- Public community: %[1]s/community
- Source, guides and licence: https://github.com/eyupio/reet.sh

## Crawling

Public pages may be indexed. App, account and private-list pages are not intended for indexing.
`

const aiTxt = `# reet.sh
# A self-hosted control room for AI agents, skills and setups across machines.
# Public pages: %[1]s/ and %[1]s/community
# Source and documentation: https://github.com/eyupio/reet.sh
`

func writeText(w http.ResponseWriter, r *http.Request, body, origin string) {
	text := fmt.Sprintf(body, html.EscapeString(origin))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeMaybeGzip(w, r, []byte(text))
}

func sitemapPaths(fn func() []string) []string {
	seen := map[string]bool{}
	paths := make([]string, 0, 32)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || p == "/" || !strings.HasPrefix(p, "/") || seen[p] {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}
	for _, p := range docsSitemapPaths() {
		add(p)
	}
	if fn != nil {
		for _, p := range fn() {
			add(p)
		}
	}
	return paths
}

func writeSitemap(w http.ResponseWriter, origin string, extra []string) {
	base := strings.TrimRight(html.EscapeString(origin), "/")
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">")
	b.WriteString("\n  <url>\n    <loc>" + base + "/</loc>\n  </url>")
	for _, p := range extra {
		b.WriteString("\n  <url>\n    <loc>" + base + html.EscapeString(p) + "</loc>\n  </url>")
	}
	b.WriteString("\n</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(b.String()))
}
