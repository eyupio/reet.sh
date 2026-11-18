package skillfetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
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

func TestFetchWellKnownRefusesTraversalAndRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-skills/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"skills":[{"name":"demo","files":["SKILL.md","../x"]}]}`))
	})
	f, r := wellKnownServer(t, mux)
	if _, err := f.Fetch(context.Background(), r); err == nil {
		t.Fatal("traversal accepted")
	}
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/.well-known/agent-skills/index.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.invalid/index.json", http.StatusFound)
	})
	f, r = wellKnownServer(t, mux2)
	if _, err := f.Fetch(context.Background(), r); err == nil {
		t.Fatal("cross-host redirect followed")
	}
}

func TestDefaultClientRefusesLoopback(t *testing.T) {
	_, r := wellKnownServer(t, demoHandler())
	_, err := Fetcher{}.Fetch(context.Background(), r)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
