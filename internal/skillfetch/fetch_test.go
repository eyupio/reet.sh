package skillfetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

type entry struct {
	name, body string
	typeflag   byte
	link       string
}

func buildTar(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: e.typeflag, Linkname: e.link}
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func githubServer(t *testing.T, archive []byte) Fetcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/o/r/tar.gz/HEAD" {
			http.NotFound(w, r)
			return
		}
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	return Fetcher{Client: srv.Client(), GitHub: srv.URL}
}

func ghRef(slug string) ref.Ref {
	return ref.Ref{ID: "o/r/" + slug, Source: "o/r", Slug: slug, SourceType: "github"}
}

func paths(files []skillbundle.ContentFile) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Path)
	}
	sort.Strings(out)
	return out
}

func TestFetchGitHubFindsSkillFolder(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{
		{name: "r-abc/", typeflag: tar.TypeDir},
		{name: "r-abc/README.md", body: "readme"},
		{name: "r-abc/skills/pdf/SKILL.md", body: "---\nname: pdf\n---\nbody"},
		{name: "r-abc/skills/pdf/ref/a.md", body: "a"},
		{name: "r-abc/skills/other/SKILL.md", body: "---\nname: other\n---\n"},
	}))
	res, err := f.Fetch(context.Background(), ghRef("pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths(res.Files), ","); got != "SKILL.md,ref/a.md" {
		t.Fatalf("files = %s", got)
	}
	if res.Origin != "https://github.com/o/r/tree/HEAD/skills/pdf" {
		t.Fatalf("origin = %s", res.Origin)
	}
	if len(res.Manifest.Files) != 2 {
		t.Fatalf("manifest = %+v", res.Manifest)
	}
}

func TestFetchGitHubMatchesFrontMatterName(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{
		{name: "r-abc/svg-logo-designer/SKILL.md", body: "---\nname: SVG Logo Designer\n---\nx"},
		{name: "r-abc/svg-logo-designer/a.txt", body: "a"},
	}))
	res, err := f.Fetch(context.Background(), ghRef("svg-logo-designer"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("files = %v", paths(res.Files))
	}
	// Name only in front matter, folder named differently.
	f = githubServer(t, buildTar(t, []entry{
		{name: "r-abc/tools/logo/SKILL.md", body: "---\nname: \"SVG Logo Designer\"\n---\nx"},
	}))
	res, err = f.Fetch(context.Background(), ghRef("svg-logo-designer"))
	if err != nil || res.Origin != "https://github.com/o/r/tree/HEAD/tools/logo" {
		t.Fatalf("res = %+v err = %v", res, err)
	}
}

func TestFetchRefusesOverLimits(t *testing.T) {
	var many []entry
	for i := 0; i < 257; i++ {
		many = append(many, entry{name: "r-abc/skills/pdf/f" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".txt", body: "x"})
	}
	many = append(many, entry{name: "r-abc/skills/pdf/SKILL.md", body: "x"})
	f := githubServer(t, buildTar(t, many))
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("257 files: err = %v", err)
	}
	big := []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "x"},
		{name: "r-abc/skills/pdf/big.bin", body: strings.Repeat("a", 9<<20)},
	}
	f = githubServer(t, buildTar(t, big))
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("9 MiB: err = %v", err)
	}
}

func TestFetchStopsReadingStream(t *testing.T) {
	// The oversized entry is refused from its header: its 9 MiB of
	// incompressible data must not be pulled off the wire.
	junk := make([]byte, 9<<20)
	rand.New(rand.NewSource(1)).Read(junk)
	body := buildTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "x"},
		{name: "r-abc/skills/pdf/big.bin", body: string(junk)},
	})
	total := len(body)
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for rest := body; len(rest) > 0; {
			n := min(len(rest), 4096)
			if _, err := w.Write(rest[:n]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			rest = rest[n:]
			sent.Add(int64(n))
		}
	}))
	f := Fetcher{Client: srv.Client(), GitHub: srv.URL}
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	srv.Close()
	if got := sent.Load(); got > int64(total)/2 {
		t.Fatalf("stream read to %d of %d bytes", got, total)
	}
}

func TestFetchRefusesCredentials(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "x"},
		{name: "r-abc/skills/pdf/.env", body: "KEY=sk-abcdefghijklmnopqrstuvwxyz123456"},
	}))
	_, err := f.Fetch(context.Background(), ghRef("pdf"))
	var blocked *skillimport.BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchNotFound(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{{name: "r-abc/skills/other/SKILL.md", body: "x"}}))
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no folder: err = %v", err)
	}
	r := ghRef("pdf")
	r.Source = "o/missing"
	if _, err := f.Fetch(context.Background(), r); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404: err = %v", err)
	}
}

func TestFetchRefusesUnsafePaths(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "x"},
		{name: "r-abc/skills/pdf/../../../etc/passwd", body: "x"},
	}))
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); err == nil {
		t.Fatal("dot-dot entry was accepted")
	}
	f = githubServer(t, buildTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "x"},
		{name: "r-abc/skills/pdf/link", typeflag: tar.TypeSymlink, link: "/etc/passwd"},
		{name: "r-abc/skills/pdf/hard", typeflag: tar.TypeLink, link: "SKILL.md"},
	}))
	res, err := f.Fetch(context.Background(), ghRef("pdf"))
	if err != nil || len(res.Files) != 1 || res.Files[0].Path != "SKILL.md" {
		t.Fatalf("links must be skipped: %v %v", paths(res.Files), err)
	}
}

func wellKnownServer(t *testing.T, handler http.Handler) (Fetcher, ref.Ref) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	return Fetcher{Client: srv.Client()}, ref.Ref{ID: host + "/demo", Source: host, Slug: "demo", SourceType: "well-known", URL: "https://www.skills.sh/site/" + host + "/demo"}
}

func demoHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-skills/index.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"skills": []any{
			map[string]any{"name": "other", "files": []string{"SKILL.md"}},
			map[string]any{"name": "demo", "description": "d", "files": []string{"SKILL.md", "references/help.md"}},
		}})
	})
	mux.HandleFunc("/.well-known/agent-skills/demo/SKILL.md", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("---\nname: demo\n---\n")) })
	mux.HandleFunc("/.well-known/agent-skills/demo/references/help.md", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("help")) })
	return mux
}

func TestFetchWellKnown(t *testing.T) {
	f, r := wellKnownServer(t, demoHandler())
	res, err := f.Fetch(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths(res.Files), ","); got != "SKILL.md,references/help.md" {
		t.Fatalf("files = %s", got)
	}
	if res.Origin != r.URL {
		t.Fatalf("origin = %s", res.Origin)
	}
	r.Slug = "missing"
	if _, err := f.Fetch(context.Background(), r); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchWellKnownRefusesTraversal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-skills/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"skills":[{"name":"demo","files":["SKILL.md","../x"]}]}`))
	})
	f, r := wellKnownServer(t, mux)
	if _, err := f.Fetch(context.Background(), r); err == nil {
		t.Fatal("traversal accepted")
	}
}

func TestFetchWellKnownRefusesOtherHostRedirect(t *testing.T) {
	var hits atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`{"skills":[]}`))
	}))
	defer target.Close()
	f, r := wellKnownServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/.well-known/agent-skills/index.json", http.StatusFound)
	}))
	_, err := f.Fetch(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "redirect to a host") {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the redirect target was contacted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchGitHubRedirectAllowlist(t *testing.T) {
	archive := buildTar(t, []entry{{name: "r-abc/skills/pdf/SKILL.md", body: "x"}})
	run := func(location string) (Result, error, []string) {
		var seen []string
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seen = append(seen, req.URL.Host)
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(archive)), Request: req}
			if req.URL.Host == "codeload.github.com" {
				resp.StatusCode = http.StatusFound
				resp.Header.Set("Location", location)
				resp.Body = io.NopCloser(strings.NewReader(""))
			}
			return resp, nil
		})}
		res, err := Fetcher{Client: client}.Fetch(context.Background(), ghRef("pdf"))
		return res, err, seen
	}
	res, err, _ := run("https://objects.githubusercontent.com/o/r")
	if err != nil || len(res.Files) != 1 {
		t.Fatalf("allowed host: %v %v", res.Files, err)
	}
	for _, loc := range []string{"https://evil.example/o/r", "http://objects.githubusercontent.com/o/r", "https://githubusercontent.com.evil.example/x"} {
		_, err, seen := run(loc)
		if err == nil || !strings.Contains(err.Error(), "redirect to a host") || len(seen) != 1 {
			t.Fatalf("%s: err = %v, hosts = %v", loc, err, seen)
		}
	}
}

func TestRefuseAddress(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "::ffff:10.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "fd00::1", "100.64.0.1", "0.0.0.0", "224.0.0.1", "fe80::1",
		"0.1.2.3", "192.0.0.8", "198.18.0.1", "198.19.255.255", "240.0.0.1", "255.255.255.255", "::ffff:240.0.0.1",
		// NAT64 and 6to4 whose embedded IPv4 is not public, and the local-use NAT64 prefix.
		"64:ff9b::7f00:1", "64:ff9b::a00:5", "64:ff9b::c0a8:101", "64:ff9b:1::1", "2002:7f00:1::", "2002:0a00:5::1", "2002:c0a8:101::1",
		"::7f00:1", "::a00:5", "::ffff:0:7f00:1", "::ffff:0:a00:5", "fec0::1", "feff::1",
		"2001:0:4136:e378:8000:63bf:80ff:fffe", "2001:0:a00:5:8000:63bf:f7f7:f7f7"} {
		if err := refuseAddress("tcp", net.JoinHostPort(ip, "443"), nil); err == nil {
			t.Errorf("%s allowed", ip)
		}
	}
	for _, ip := range []string{"140.82.112.3", "2606:50c0:8000::154", "198.20.0.1", "223.255.255.1", "64:ff9b::8c52:7003", "2002:8c52:7003::1", "::8c52:7003", "::ffff:0:8c52:7003", "2001:0:4136:e378:8000:63bf:f7f7:f7f7"} {
		if err := refuseAddress("tcp", net.JoinHostPort(ip, "443"), nil); err != nil {
			t.Errorf("%s refused: %v", ip, err)
		}
	}
}

func TestFetchEarlyStopIgnoresUnrelatedSlugFolders(t *testing.T) {
	var noise, skill []entry
	for i := 0; i < 300; i++ {
		noise = append(noise, entry{name: "r-abc/packages/pdf/f" + strings.Repeat("y", i/26) + string(rune('a'+i%26)) + ".js", body: "x"})
	}
	skill = []entry{{name: "r-abc/skills/pdf/SKILL.md", body: "x"}}
	for name, entries := range map[string][]entry{"noise first": append(append([]entry{}, noise...), skill...), "skill first": append(append([]entry{}, skill...), noise...)} {
		f := githubServer(t, buildTar(t, entries))
		res, err := f.Fetch(context.Background(), ghRef("pdf"))
		if err != nil || len(res.Files) != 1 || res.Origin != "https://github.com/o/r/tree/HEAD/skills/pdf" {
			t.Fatalf("%s: %+v %v", name, res, err)
		}
	}
}

func TestDefaultClientRefusesLoopback(t *testing.T) {
	_, r := wellKnownServer(t, demoHandler())
	_, err := Fetcher{}.Fetch(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "not one this server may connect to") {
		t.Fatalf("err = %v", err)
	}
}

func proxyFixture(t *testing.T, handler http.HandlerFunc) (*atomic.Int64, func(*http.Request) (*url.URL, error)) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return &hits, func(*http.Request) (*url.URL, error) { return u, nil }
}

func fakeLookup(answers map[string]string) func(context.Context, string) ([]net.IP, error) {
	return func(_ context.Context, host string) ([]net.IP, error) {
		if a, ok := answers[host]; ok {
			return []net.IP{net.ParseIP(a)}, nil
		}
		return nil, errors.New("no such host")
	}
}

func TestProxyReachedForPublicDestination(t *testing.T) {
	var method, target atomic.Value
	hits, proxy := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		method.Store(r.Method)
		target.Store(r.Host)
		http.Error(w, "stand-in proxy", http.StatusBadGateway)
	})
	f := Fetcher{proxy: proxy, lookup: fakeLookup(map[string]string{"example.test": "140.82.112.3"})}
	r := ref.Ref{Source: "example.test", Slug: "demo", SourceType: "well-known"}
	_, err := f.Fetch(context.Background(), r)
	if err == nil || strings.Contains(err.Error(), "may connect to") {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() == 0 || method.Load() != "CONNECT" || target.Load() != "example.test:443" {
		t.Fatalf("proxy hits=%d method=%v target=%v", hits.Load(), method.Load(), target.Load())
	}
}

func TestProxyRefusesNonPublicDestinationBeforeContact(t *testing.T) {
	for _, source := range []string{"127.0.0.1:8443", "10.0.0.5", "internal.test"} {
		hits, proxy := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {})
		f := Fetcher{proxy: proxy, lookup: fakeLookup(map[string]string{"internal.test": "10.1.2.3"})}
		r := ref.Ref{Source: source, Slug: "demo", SourceType: "well-known"}
		_, err := f.Fetch(context.Background(), r)
		if err == nil || !strings.Contains(err.Error(), "may connect to") || hits.Load() != 0 {
			t.Fatalf("%s: err = %v, proxy hits = %d", source, err, hits.Load())
		}
	}
}

func TestProxyRefusesRedirectToPrivateAddress(t *testing.T) {
	hits, proxy := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://objects.githubusercontent.com/x", http.StatusFound)
	})
	f := Fetcher{
		GitHub: "http://example.test", proxy: proxy,
		lookup: fakeLookup(map[string]string{"example.test": "140.82.112.3", "objects.githubusercontent.com": "10.0.0.5"}),
	}
	_, err := f.Fetch(context.Background(), ghRef("pdf"))
	if err == nil || !strings.Contains(err.Error(), "may connect to") || hits.Load() != 1 {
		t.Fatalf("err = %v, proxy hits = %d", err, hits.Load())
	}
}

func TestFetchRefusesAbsolutePathEntry(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "x"},
		{name: "/etc/passwd", body: "root"},
	}))
	_, err := f.Fetch(context.Background(), ghRef("pdf"))
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchRefusesUnknownSourceTypeAndEmptySlug(t *testing.T) {
	f := githubServer(t, buildTar(t, []entry{{name: "r-abc/skills/pdf/SKILL.md", body: "x"}}))
	for _, st := range []string{"hosted", "", "gitlab"} {
		r := ghRef("pdf")
		r.SourceType = st
		_, err := f.Fetch(context.Background(), r)
		if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "source type") {
			t.Fatalf("%q: err = %v", st, err)
		}
	}
	r := ghRef("")
	if _, err := f.Fetch(context.Background(), r); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("empty slug: err = %v", err)
	}
}

func TestFetchWellKnownRefusesUnsafeSkillName(t *testing.T) {
	for _, name := range []string{"..", ".", "a/b", `a\b`} {
		var hits atomic.Int64
		mux := http.NewServeMux()
		mux.HandleFunc("/.well-known/agent-skills/index.json", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"skills": []any{map[string]any{"name": name, "files": []string{"SKILL.md"}}}})
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "index.json") {
				hits.Add(1)
			}
			w.Write([]byte("---\nname: x\n---\n"))
		})
		f, r := wellKnownServer(t, mux)
		r.Slug = name
		if name == ".." || name == "." || strings.ContainsAny(name, "/\\") {
			r.Slug = normalize(name)
		}
		_, err := f.Fetch(context.Background(), r)
		if err == nil || hits.Load() != 0 {
			t.Fatalf("%q: err = %v, file requests = %d", name, err, hits.Load())
		}
	}
}

func TestFetchWellKnownTooLarge(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-skills/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"skills":[{"name":"demo","files":["SKILL.md","big.bin"]}]}`))
	})
	mux.HandleFunc("/.well-known/agent-skills/demo/SKILL.md", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) })
	mux.HandleFunc("/.well-known/agent-skills/demo/big.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, skillbundle.MaxBytes+1))
	})
	f, r := wellKnownServer(t, mux)
	if _, err := f.Fetch(context.Background(), r); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a file over the limit: err = %v", err)
	}
	var names []string
	for i := 0; i <= skillbundle.MaxFiles; i++ {
		names = append(names, "f"+strings.Repeat("a", i%5)+string(rune('a'+i%26))+string(rune('a'+i/26))+".txt")
	}
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/.well-known/agent-skills/index.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"skills": []any{map[string]any{"name": "demo", "files": names}}})
	})
	f, r = wellKnownServer(t, mux2)
	if _, err := f.Fetch(context.Background(), r); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too many files: err = %v", err)
	}
}

func TestFetchFolderPreferenceOrder(t *testing.T) {
	skill := func(dir, name string) []entry {
		return []entry{{name: "r-abc/" + dir + "/SKILL.md", body: "---\nname: " + name + "\n---\n"}, {name: "r-abc/" + dir + "/from.txt", body: dir}}
	}
	concat := func(parts ...[]entry) []entry {
		var out []entry
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	for _, tc := range []struct {
		name    string
		entries []entry
		want    string
	}{
		{"skills/<slug> beats an exact folder name", concat(skill("tools/pdf", "pdf"), skill("skills/pdf", "other")), "skills/pdf"},
		{"an exact folder name beats a front-matter name", concat(skill("a/x", "pdf"), skill("z/pdf", "other")), "z/pdf"},
		{"a front-matter name beats a loose folder name", concat(skill("a/PDF", "other"), skill("z/x", "pdf")), "z/x"},
		{"a loose folder name matches when nothing else does", skill("a/PDF", "other"), "a/PDF"},
		{"the shallowest of equals", concat(skill("a/b/pdf", "pdf"), skill("c/pdf", "pdf")), "c/pdf"},
		{"then the first by name", concat(skill("z/pdf", "pdf"), skill("b/pdf", "pdf")), "b/pdf"},
	} {
		res, err := githubServer(t, buildTar(t, tc.entries)).Fetch(context.Background(), ghRef("pdf"))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if res.Origin != "https://github.com/o/r/tree/HEAD/"+tc.want {
			t.Errorf("%s: origin = %s, want folder %s", tc.name, res.Origin, tc.want)
		}
	}
}

func TestProxyRefusesMixedPublicAndPrivateAnswers(t *testing.T) {
	hits, proxy := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {})
	f := Fetcher{proxy: proxy, lookup: func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("140.82.112.3"), net.ParseIP("10.0.0.5")}, nil
	}}
	r := ref.Ref{Source: "mixed.test", Slug: "demo", SourceType: "well-known"}
	_, err := f.Fetch(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "may connect to") || hits.Load() != 0 {
		t.Fatalf("err = %v, proxy hits = %d", err, hits.Load())
	}
}

func TestProxyRefusesEmptyAnswer(t *testing.T) {
	hits, proxy := proxyFixture(t, func(w http.ResponseWriter, r *http.Request) {})
	f := Fetcher{proxy: proxy, lookup: func(context.Context, string) ([]net.IP, error) { return nil, nil }}
	r := ref.Ref{Source: "empty.test", Slug: "demo", SourceType: "well-known"}
	_, err := f.Fetch(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "may connect to") || hits.Load() != 0 {
		t.Fatalf("err = %v, proxy hits = %d", err, hits.Load())
	}
}

// The proxy's own address is exempt from the dial guard only for the proxy
// leg. A direct dial (NO_PROXY) to that same loopback address stays refused.
func TestDirectDialToTheProxyAddressStaysGuarded(t *testing.T) {
	var proxied atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	pu, _ := url.Parse(srv.URL)
	proxy := func(req *http.Request) (*url.URL, error) {
		if req.URL.Host == "example.test" {
			return pu, nil
		}
		return nil, nil // NO_PROXY
	}
	c := defaultClient(proxy, fakeLookup(map[string]string{"example.test": "140.82.112.3"}))
	resp, err := c.Get("http://example.test/")
	if err != nil {
		t.Fatalf("through the proxy: %v", err)
	}
	resp.Body.Close()
	if proxied.Load() != 1 {
		t.Fatalf("the proxy was not used: %d", proxied.Load())
	}
	_, err = c.Get(srv.URL + "/")
	if err == nil || !strings.Contains(err.Error(), "may connect to") || proxied.Load() != 1 {
		t.Fatalf("direct dial to the proxy's address: err = %v, hits = %d", err, proxied.Load())
	}
}

func TestFetchHasAnOverallDeadline(t *testing.T) {
	// Every body trickles, so no single request is slow, but the whole Fetch
	// would take far longer than the limit.
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "index.json") {
			w.Write([]byte(`{"skills":[{"name":"demo","files":["SKILL.md"]}]}`))
			return
		}
		w.WriteHeader(200)
		for {
			select {
			case <-r.Context().Done():
				return
			case <-release:
				return
			case <-time.After(10 * time.Millisecond):
				w.Write([]byte("x"))
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer srv.Close()
	defer close(release)
	host := strings.TrimPrefix(srv.URL, "https://")
	f := Fetcher{Client: srv.Client(), timeout: 200 * time.Millisecond}
	start := time.Now()
	_, err := f.Fetch(context.Background(), ref.Ref{Source: host, Slug: "demo", SourceType: "well-known"})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatalf("err = %v after %v", err, time.Since(start))
	}
	// A shorter deadline of the caller still wins.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	f.timeout = time.Hour
	start = time.Now()
	if _, err = f.Fetch(ctx, ref.Ref{Source: host, Slug: "demo", SourceType: "well-known"}); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Fatalf("caller deadline: err = %v after %v", err, time.Since(start))
	}
}

func TestFetchBuildsOneClientAndClosesItsConnections(t *testing.T) {
	var built, closed atomic.Int64
	srv := httptest.NewTLSServer(demoHandler())
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	f := Fetcher{newClient: func() *http.Client {
		built.Add(1)
		c := srv.Client()
		tr := c.Transport.(*http.Transport).Clone()
		c = &http.Client{Transport: closeCounter{tr, &closed}}
		return c
	}}
	r := ref.Ref{Source: host, Slug: "demo", SourceType: "well-known", URL: "https://www.skills.sh/x"}
	if _, err := f.Fetch(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if built.Load() != 1 || closed.Load() != 1 {
		t.Fatalf("a Fetch of 3 requests built %d clients and closed idle connections %d times", built.Load(), closed.Load())
	}
}

type closeCounter struct {
	*http.Transport
	n *atomic.Int64
}

func (c closeCounter) CloseIdleConnections() { c.n.Add(1); c.Transport.CloseIdleConnections() }

// gzipTar is buildTar with the fastest compression, for big, repetitive
// bodies. A regular entry carries its body; any other type carries size zero
// bytes of data, the way a reader that does not know the type will skip them.
func gzipTar(t *testing.T, entries []entry, size int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		tf, n := e.typeflag, int64(len(e.body))
		if tf == 0 {
			tf = tar.TypeReg
		}
		if tf != tar.TypeReg {
			n = size
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o644, Typeflag: tf, Size: n}); err != nil {
			t.Fatal(err)
		}
		var src io.Reader = strings.NewReader(e.body)
		if tf != tar.TypeReg {
			src = io.LimitReader(zeroReader{}, n)
		}
		if _, err := io.Copy(tw, src); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestFetchCountsNonRegularEntrySizesTowardTheStreamCap(t *testing.T) {
	body := gzipTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "x"},
		{name: "r-abc/pad", typeflag: 'Z'}, // a type the reader hands over with data
	}, 64<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), GitHub: srv.URL, streamCap: 32 << 10}
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

// Two sizes whose sum wraps past MaxInt64 must not slip under the cap.
func TestFetchStreamCapSurvivesSizeOverflow(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "r-abc/a", Mode: 0o644, Typeflag: 'Z', Size: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(make([]byte, 100)); err != nil {
		t.Fatal(err)
	}
	// Only the header: the body is never sent, so a reader that tries to skip
	// it gets an unexpected EOF instead of the size error.
	if err := tw.WriteHeader(&tar.Header{Name: "r-abc/b", Mode: 0o644, Typeflag: 'Z', Size: math.MaxInt64 - 50}); err != nil {
		t.Fatal(err)
	}
	gz.Close()
	body := buf.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), GitHub: srv.URL}
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

// The body a regular entry is built with must reach the archive.
func TestGzipTarKeepsRegularBodies(t *testing.T) {
	body := gzipTar(t, []entry{{name: "r-abc/skills/pdf/SKILL.md", body: "---\nname: pdf\n---\nhello"}}, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	res, err := Fetcher{Client: srv.Client(), GitHub: srv.URL}.Fetch(context.Background(), ghRef("pdf"))
	if err != nil || !strings.Contains(string(res.Files[0].Data), "hello") {
		t.Fatalf("%v %+v", err, res.Files)
	}
}

func TestFetchTruncatedArchiveIsTooLarge(t *testing.T) {
	junk := make([]byte, 64<<10)
	rand.New(rand.NewSource(2)).Read(junk)
	body := buildTar(t, []entry{
		{name: "r-abc/skills/pdf/SKILL.md", body: "x"},
		{name: "r-abc/skills/pdf/a.bin", body: string(junk)},
		{name: "r-abc/skills/other/SKILL.md", body: "y"},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), GitHub: srv.URL, archiveCap: 16 << 10}
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("a cut archive must say too large, not a gzip error: %v", err)
	}
	f.archiveCap = int64(len(body)) // exactly its size is fine
	if _, err := f.Fetch(context.Background(), ghRef("pdf")); err != nil {
		t.Fatalf("an archive at the cap: %v", err)
	}
}
