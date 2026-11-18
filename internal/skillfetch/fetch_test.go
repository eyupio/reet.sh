package skillfetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

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
	for _, ip := range []string{"127.0.0.1", "::1", "::ffff:10.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "fd00::1", "100.64.0.1", "0.0.0.0", "224.0.0.1", "fe80::1"} {
		if err := refuseAddress("tcp", net.JoinHostPort(ip, "443"), nil); err == nil {
			t.Errorf("%s allowed", ip)
		}
	}
	for _, ip := range []string{"140.82.112.3", "2606:50c0:8000::154"} {
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
